package com.telecyborg.data

import com.telecyborg.core.Hash
import com.telecyborg.core.Meta
import com.telecyborg.core.Pack
import com.telecyborg.core.Packs
import com.telecyborg.core.Parsed
import com.telecyborg.core.Script
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.put
import java.io.File
import java.time.Instant

val TcJson = Json { ignoreUnknownKeys = true; encodeDefaults = true; prettyPrint = false }

/** Preferences that sync with the PC (same JSON keys as Go's hub.SyncedSettings). */
@Serializable
data class SyncedSettings(
    val wpm: Int = 140,
    val fontSize: Int = 44,
    val mirror: Boolean = false,
    val focusLine: Double = 0.38,
    val countdown: Int = 3,
    val motion: String = "full",
    val answerMax: Int = 60,
)

data class Summary(
    val id: String, val title: String, val words: Int, val paragraphs: Int, val wpm: Int, val target: Double,
    val duration: Double, val modified: Long, val runs: Int, val qa: Int, val pack: String,
)

data class ScriptDoc(val id: String, val source: String, val body: String, val parsed: Parsed) {
    val title get() = parsed.meta.title
    val wpm get() = parsed.meta.wpm
    val target get() = parsed.meta.target
}

/**
 * Everything the phone keeps, laid out like the PC so sync is a file exchange:
 * scripts/<id>.md (Markdown, verbatim), packs/<id>.json (the sync content of a pack:
 * {"pack":…,"answer":…}), rehearsals/<id>.json (JSON array), synced.json (synced settings).
 * For every item the file text is exactly what is hashed and exchanged.
 */
class Repo(val root: File) {
    private val scripts = File(root, "scripts")
    private val packs = File(root, "packs")
    private val runs = File(root, "rehearsals")
    private val synced = File(root, "synced.json")

    init { listOf(root, scripts, packs, runs).forEach { it.mkdirs() } }

    @Synchronized private fun write(f: File, text: String) {
        f.parentFile?.mkdirs()
        val tmp = File(f.path + ".tmp")
        tmp.writeText(text)
        if (!tmp.renameTo(f)) { f.delete(); tmp.renameTo(f) }
    }

    // ---------- scripts ----------
    fun scriptIds(): List<String> = scripts.listFiles { f -> f.name.endsWith(".md") }?.map { it.name.removeSuffix(".md") }?.filter(Script::validId) ?: emptyList()

    fun list(): List<Summary> = scriptIds().mapNotNull { id ->
        val f = File(scripts, "$id.md")
        val p = runCatching { Script.parse(f.readText()) }.getOrNull() ?: return@mapNotNull null
        Summary(id, p.meta.title, p.words, p.paragraphs, p.meta.wpm, p.meta.target, p.duration(), f.lastModified(), runs(id).size,
            p.qaItems, p.meta.extra["pack"] ?: "")
    }.sortedByDescending { it.modified }

    fun get(id: String): ScriptDoc? {
        if (!Script.validId(id)) return null
        val f = File(scripts, "$id.md")
        if (!f.exists()) return null
        val src = f.readText()
        return ScriptDoc(id, src, Script.splitFrontMatter(src).second, Script.parse(src))
    }

    private fun uniqueId(title: String, dir: File, ext: String): String {
        val base = Script.slug(title)
        var id = base
        var i = 2
        while (File(dir, id + ext).exists()) { id = "$base-$i"; i++ }
        return id
    }

    /** Create (id == null) or update a script from the editor, keeping other front-matter keys. */
    fun saveScript(id: String?, title: String, body: String, wpm: Int, target: Double): String {
        val keep = (id?.let { get(it)?.parsed?.meta } ?: Script.parse(body).meta)
        var m: Meta = keep.copy(title = title.trim(), wpm = if (wpm > 0) wpm else keep.wpm, target = target)
        if (m.title.isEmpty()) m = m.copy(title = Script.parse(body).meta.title)
        val src = Script.compose(m, body)
        val newId = id ?: uniqueId(Script.parse(src).meta.title, scripts, ".md")
        write(File(scripts, "$newId.md"), src)
        return newId
    }

    /** Import a whole file (front matter kept). */
    fun importScript(name: String, text: String): String {
        val p = Script.parse(text)
        val title = p.meta.title.ifEmpty { name.substringBeforeLast('.').replace(Regex("[_-]+"), " ").trim() }
        return saveScript(null, title, text, p.meta.wpm, p.meta.target)
    }

    fun putScriptSource(id: String, source: String) = write(File(scripts, "$id.md"), source)

    fun deleteScript(id: String) {
        File(scripts, "$id.md").delete()
        File(runs, "$id.json").delete()
    }

    // ---------- rehearsals (kept as the PC's JSON objects, so PC-only fields survive) ----------
    fun runs(id: String): List<JsonObject> {
        val f = File(runs, "$id.json")
        if (!f.exists()) return emptyList()
        return runCatching { (TcJson.parseToJsonElement(f.readText()) as JsonArray).map { it.jsonObject } }.getOrDefault(emptyList())
    }

    fun addRun(id: String, run: JsonObject) {
        val withAt = if (run["at"] == null) JsonObject(run + ("at" to JsonPrimitive(Instant.now().toString().substringBefore('.') + "Z"))) else run
        write(File(runs, "$id.json"), JsonArray(mergeRuns(listOf(withAt), runs(id))).toString())
    }

    // ---------- packs ----------
    fun packIds(): List<String> = packs.listFiles { f -> f.name.endsWith(".json") }?.map { it.name.removeSuffix(".json") }?.filter(Script::validId) ?: emptyList()

    data class PackDoc(val pack: Pack, val answer: String)

    fun pack(id: String): PackDoc? {
        val f = File(packs, "$id.json")
        if (!f.exists()) return null
        return runCatching {
            val o = TcJson.parseToJsonElement(f.readText()).jsonObject
            val p = TcJson.decodeFromJsonElement(Pack.serializer(), o["pack"] ?: JsonObject(emptyMap())).copy(id = id)
            PackDoc(p, o["answer"]?.jsonPrimitive?.contentOrNull ?: "")
        }.getOrNull()
    }

    fun listPacks(): List<Pack> = packIds().mapNotNull { pack(it)?.pack }.sortedByDescending { it.modified }

    fun savePack(p0: Pack, answer: String? = null): Pack {
        val now = Instant.now().toString().substringBefore('.') + "Z"
        var p = Packs.normalize(p0)
        val old = if (p.id.isNotEmpty()) pack(p.id) else null
        if (p.id.isEmpty()) p = p.copy(id = uniqueId(p.title, packs, ".json"), created = now)
        else if (old != null) p = p.copy(created = old.pack.created, scriptId = p.scriptId.ifEmpty { old.pack.scriptId })
        p = p.copy(modified = now)
        writePack(p, answer ?: old?.answer ?: "")
        return p
    }

    private fun writePack(p: Pack, answer: String) {
        val content = buildJsonObject {
            put("pack", TcJson.encodeToJsonElement(Pack.serializer(), p))
            put("answer", answer)
        }
        write(File(packs, "${p.id}.json"), content.toString())
    }

    /** Validate Claude's answer and create or update the pack's script. */
    fun importAnswer(packId: String, answer: String): Pair<String?, com.telecyborg.core.Preview> {
        val doc = pack(packId) ?: return null to Packs.validate(answer, Pack())
        val pv = Packs.validate(answer, doc.pack)
        if (!pv.ok) return null to pv
        val body = Packs.withPack(pv.body, doc.pack)
        val sid = doc.pack.scriptId.takeIf { it.isNotEmpty() && get(it) != null } ?: uniqueId(pv.title, scripts, ".md")
        putScriptSource(sid, body)
        writePack(doc.pack.copy(scriptId = sid, modified = Instant.now().toString().substringBefore('.') + "Z"), answer)
        return sid to pv
    }

    fun deletePack(id: String) { File(packs, "$id.json").delete() }

    // ---------- settings ----------
    fun settings(): SyncedSettings = runCatching { TcJson.decodeFromString(SyncedSettings.serializer(), synced.readText()) }.getOrDefault(SyncedSettings())

    fun saveSettings(s: SyncedSettings) = write(synced, TcJson.encodeToString(SyncedSettings.serializer(), s))

    // ---------- sync view ----------
    fun file(kind: String, id: String): File? = when (kind) {
        KIND_SCRIPT -> File(scripts, "$id.md")
        KIND_PACK -> File(packs, "$id.json")
        KIND_REHEARSALS -> File(runs, "$id.json")
        KIND_SETTINGS -> if (id == "prefs") synced else null
        else -> null
    }

    /** Content of an item, exactly as exchanged (null when absent). */
    fun content(kind: String, id: String): String? = file(kind, id)?.takeIf { it.exists() }?.readText()

    fun putContent(kind: String, id: String, content: String) { file(kind, id)?.let { write(it, content) } }

    fun deleteContent(kind: String, id: String) { file(kind, id)?.delete() }

    fun modified(kind: String, id: String): Long = file(kind, id)?.takeIf { it.exists() }?.lastModified() ?: 0

    /** kind/id → hash for everything on the phone. */
    fun manifest(): Map<String, String> {
        val out = LinkedHashMap<String, String>()
        fun add(kind: String, id: String) { content(kind, id)?.let { out["$kind/$id"] = Hash.of(it) } }
        scriptIds().forEach { add(KIND_SCRIPT, it) }
        packIds().forEach { add(KIND_PACK, it) }
        runs.listFiles { f -> f.name.endsWith(".json") }?.forEach { add(KIND_REHEARSALS, it.name.removeSuffix(".json")) }
        add(KIND_SETTINGS, "prefs")
        return out
    }

    companion object {
        const val KIND_SCRIPT = "script"
        const val KIND_PACK = "pack"
        const val KIND_SETTINGS = "settings"
        const val KIND_REHEARSALS = "rehearsals"

        /** Union of two rehearsal histories by "at", newest first, at most 20 (same as the PC). */
        fun mergeRuns(a: List<JsonElement>, b: List<JsonElement>): List<JsonObject> {
            val seen = HashSet<String>()
            val out = ArrayList<JsonObject>()
            for (e in a + b) {
                val o = e as? JsonObject ?: continue
                val at = o["at"]?.jsonPrimitive?.contentOrNull ?: continue
                if (at.isEmpty() || !seen.add(at)) continue
                out.add(o)
            }
            return out.sortedByDescending { it["at"]!!.jsonPrimitive.content }.take(20)
        }
    }
}
