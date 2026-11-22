package com.telecyborg.sync

import com.telecyborg.core.Hash
import com.telecyborg.core.Script
import com.telecyborg.data.Repo
import com.telecyborg.data.Repo.Companion.KIND_PACK
import com.telecyborg.data.Repo.Companion.KIND_REHEARSALS
import com.telecyborg.data.Repo.Companion.KIND_SCRIPT
import com.telecyborg.data.Repo.Companion.KIND_SETTINGS
import com.telecyborg.data.TcJson
import kotlinx.serialization.Serializable
import kotlinx.serialization.builtins.MapSerializer
import kotlinx.serialization.builtins.serializer
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.jsonObject
import java.io.File

/** What the phone remembers about its PC and the last sync. */
@Serializable
data class SyncState(val pc: PcLink? = null, val base: Map<String, String> = emptyMap(), val lastSync: Long = 0, val lastResult: String = "")

class SyncStore(private val file: File) {
    fun load(): SyncState = runCatching { TcJson.decodeFromString(SyncState.serializer(), file.readText()) }.getOrDefault(SyncState())
    fun save(s: SyncState) {
        file.parentFile?.mkdirs()
        file.writeText(TcJson.encodeToString(SyncState.serializer(), s))
    }
}

/** The PC as the sync engine sees it (HubClient in the app, a fake in tests). */
interface Remote {
    fun manifest(): List<Entry>
    fun get(kind: String, id: String): Item
    fun put(kind: String, id: String, base: String, content: String): Item
    fun delete(kind: String, id: String, base: String)
}

fun HubClient.asRemote() = object : Remote {
    override fun manifest() = this@asRemote.manifest()
    override fun get(kind: String, id: String) = this@asRemote.get(kind, id)
    override fun put(kind: String, id: String, base: String, content: String) = this@asRemote.put(kind, id, base, content)
    override fun delete(kind: String, id: String, base: String) = this@asRemote.delete(kind, id, base)
}

data class SyncReport(var pulled: Int = 0, var pushed: Int = 0, var deletedHere: Int = 0, var deletedThere: Int = 0, val copies: MutableList<String> = mutableListOf()) {
    override fun toString(): String {
        val parts = buildList {
            if (pulled > 0) add("$pulled from PC")
            if (pushed > 0) add("$pushed to PC")
            if (deletedHere + deletedThere > 0) add("${deletedHere + deletedThere} deleted")
            if (copies.isNotEmpty()) add("${copies.size} conflict copy")
        }
        return if (parts.isEmpty()) "Up to date" else parts.joinToString(" · ")
    }
}

/**
 * Hub-and-spoke three-way sync. For every item it compares the phone's hash (L), the PC's (R)
 * and the hash both had at the last sync (B):
 *  - L == R: nothing to do.
 *  - only the PC changed (L == B): take the PC's version (or delete here).
 *  - only the phone changed (R == B): send ours (or delete there).
 *  - both changed: rehearsals merge; settings keep the newer; a script or pack keeps the PC's
 *    version under its id and saves the phone's version as "(phone copy)", so nothing is lost.
 */
class SyncEngine(private val repo: Repo, private val remote: Remote, private val base: MutableMap<String, String>) {
    val report = SyncReport()

    fun run(): SyncReport {
        val local = repo.manifest()
        val remoteEntries = remote.manifest().associateBy { it.kind + "/" + it.id }
        val keys = LinkedHashSet<String>().apply { addAll(local.keys); addAll(remoteEntries.keys); addAll(base.keys) }
        for (key in keys) {
            val kind = key.substringBefore('/')
            val id = key.substringAfter('/')
            if (kind !in setOf(KIND_SCRIPT, KIND_PACK, KIND_SETTINGS, KIND_REHEARSALS)) continue
            val re = remoteEntries[key]
            sync(kind, id, local[key], re?.takeIf { !it.deleted }?.hash?.ifEmpty { null }, base[key], re)
        }
        return report
    }

    private fun sync(kind: String, id: String, l: String?, r: String?, b: String?, re: Entry?) {
        val key = "$kind/$id"
        when {
            l == r -> if (l == null) base.remove(key) else base[key] = l
            l == b -> if (r == null) { repo.deleteContent(kind, id); base.remove(key); report.deletedHere++ } else pull(kind, id)
            r == b -> if (l == null) { remote.delete(kind, id, b ?: ""); base.remove(key); report.deletedThere++ } else push(kind, id, r ?: "")
            else -> conflict(kind, id, l, r, re)
        }
    }

    private fun pull(kind: String, id: String) {
        val it = remote.get(kind, id)
        if (it.deleted) { repo.deleteContent(kind, id); base.remove("$kind/$id"); report.deletedHere++; return }
        repo.putContent(kind, id, it.content)
        base["$kind/$id"] = Hash.of(it.content)
        report.pulled++
    }

    private fun push(kind: String, id: String, remoteBase: String) {
        val content = repo.content(kind, id) ?: return
        try {
            val stored = remote.put(kind, id, remoteBase, content)
            // the PC may normalise packs, settings and merged rehearsals: keep its version
            if (stored.content.isNotEmpty() && stored.content != content) repo.putContent(kind, id, stored.content)
            base["$kind/$id"] = Hash.of(repo.content(kind, id) ?: content)
            report.pushed++
        } catch (e: HubException) {
            if (e.code == 409) conflict(kind, id, Hash.of(content), e.current?.hash, null) else throw e
        }
    }

    private fun conflict(kind: String, id: String, l: String?, r: String?, re: Entry?) {
        val key = "$kind/$id"
        when (kind) {
            KIND_REHEARSALS -> {
                val mine = repo.content(kind, id)?.let { runCatching { TcJson.parseToJsonElement(it) as JsonArray }.getOrNull() } ?: JsonArray(emptyList())
                val theirs = if (r != null) runCatching { TcJson.parseToJsonElement(remote.get(kind, id).content) as JsonArray }.getOrNull() ?: JsonArray(emptyList()) else JsonArray(emptyList())
                val merged = JsonArray(Repo.mergeRuns(mine, theirs)).toString()
                val stored = remote.put(kind, id, r ?: "", merged)
                repo.putContent(kind, id, stored.content.ifEmpty { merged })
                base[key] = Hash.of(repo.content(kind, id)!!)
                report.pushed++
            }
            KIND_SETTINGS -> {
                val remoteNewer = (re?.modified ?: 0) > repo.modified(kind, id)
                if (remoteNewer || l == null) pull(kind, id) else push(kind, id, r ?: "")
            }
            else -> when {
                l == null -> pull(kind, id) // deleted here, edited on the PC: keep the edit
                r == null -> { // deleted on the PC, edited here: put it back
                    val stored = remote.put(kind, id, "", repo.content(kind, id)!!)
                    base[key] = Hash.of(stored.content.ifEmpty { repo.content(kind, id)!! })
                    report.pushed++
                }
                else -> {
                    val copyId = copyOf(kind, id)
                    pull(kind, id)
                    if (copyId != null) {
                        report.copies.add(copyId)
                        val c = repo.content(kind, copyId)!!
                        val stored = remote.put(kind, copyId, "", c)
                        base["$kind/$copyId"] = Hash.of(stored.content.ifEmpty { c })
                        report.pushed++
                    }
                }
            }
        }
    }

    /** Saves the phone's version of a script or pack under a new id, marked as a phone copy. */
    private fun copyOf(kind: String, id: String): String? {
        val content = repo.content(kind, id) ?: return null
        var n = 1
        var copyId: String
        do { copyId = if (n == 1) "$id-phone" else "$id-phone-$n"; n++ } while (repo.content(kind, copyId) != null)
        val text = when (kind) {
            KIND_SCRIPT -> {
                val (fm, body) = Script.splitFrontMatter(content)
                val title = (fm["title"] ?: Script.parse(content).meta.title) + " (phone copy)"
                fm["title"] = title
                "---\n" + fm.entries.joinToString("") { "${it.key}: ${it.value}\n" } + "---\n" + body.trimStart('\n')
            }
            KIND_PACK -> runCatching {
                val o = TcJson.parseToJsonElement(content).jsonObject
                val p = o["pack"]!!.jsonObject
                val t = (p["title"] as? JsonPrimitive)?.content ?: id
                JsonObject(o + ("pack" to JsonObject(p + mapOf("title" to JsonPrimitive("$t (phone copy)"), "id" to JsonPrimitive(copyId), "scriptId" to JsonPrimitive(""))))).toString()
            }.getOrDefault(content)
            else -> content
        }
        repo.putContent(kind, copyId, text)
        return copyId
    }

    companion object {
        val baseSerializer = MapSerializer(String.serializer(), String.serializer())
    }
}
