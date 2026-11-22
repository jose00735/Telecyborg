package com.telecyborg

import com.telecyborg.core.Hash
import com.telecyborg.data.Repo
import com.telecyborg.data.SyncedSettings
import com.telecyborg.sync.Entry
import com.telecyborg.sync.HubClient
import com.telecyborg.sync.HubException
import com.telecyborg.sync.Item
import com.telecyborg.sync.Remote
import com.telecyborg.sync.SyncEngine
import com.telecyborg.sync.asRemote
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Assume.assumeTrue
import org.junit.Test
import java.io.BufferedReader
import java.io.File
import java.io.InputStreamReader
import java.nio.file.Files
import java.security.SecureRandom

/** An in-memory PC with the hub's rules (conflict on a stale base for scripts and packs). */
class FakePc : Remote {
    val items = LinkedHashMap<String, String>()
    val tombs = HashSet<String>()
    override fun manifest() = items.map { (k, v) -> Entry(k.substringBefore('/'), k.substringAfter('/'), Hash.of(v), 1) } +
        tombs.map { Entry(it.substringBefore('/'), it.substringAfter('/'), "", 1, deleted = true) }
    override fun get(kind: String, id: String) = items["$kind/$id"]?.let { Item(kind, id, Hash.of(it), 1, false, it) } ?: Item(kind, id, deleted = true)
    override fun put(kind: String, id: String, base: String, content: String): Item {
        val cur = items["$kind/$id"]
        if (cur != null && (kind == "script" || kind == "pack") && Hash.of(cur) != base && cur != content) throw HubException(409, "conflict", get(kind, id))
        items["$kind/$id"] = content; tombs.remove("$kind/$id")
        return get(kind, id)
    }
    override fun delete(kind: String, id: String, base: String) {
        val cur = items["$kind/$id"] ?: return
        if (Hash.of(cur) != base) throw HubException(409, "conflict", get(kind, id))
        items.remove("$kind/$id"); tombs.add("$kind/$id")
    }
}

class SyncTest {
    private fun repo() = Repo(Files.createTempDirectory("tc").toFile())

    @Test
    fun threeWaySync() {
        val phone = repo()
        val pc = FakePc()
        val base = HashMap<String, String>()
        pc.items["script/pc-pitch"] = "---\ntitle: PC pitch\n---\n# A\nHello.\n"
        val phoneId = phone.saveScript(null, "Phone pitch", "# B\nFrom the phone.", 140, 0.0)
        SyncEngine(phone, pc, base).run().also { assertEquals(1, it.pulled); assertEquals(1, it.pushed) }
        assertNotNull(phone.get("pc-pitch"))
        assertTrue(pc.items.containsKey("script/$phoneId"))
        assertEquals(0, SyncEngine(phone, pc, base).run().let { it.pulled + it.pushed })

        // edit on the PC only → pulled; edit on the phone only → pushed
        pc.items["script/pc-pitch"] = "---\ntitle: PC pitch\n---\n# A\nHello again.\n"
        phone.saveScript(phoneId, "Phone pitch", "# B\nEdited on the phone.", 150, 0.0)
        val r = SyncEngine(phone, pc, base).run()
        assertEquals(1, r.pulled); assertEquals(1, r.pushed)
        assertTrue(phone.get("pc-pitch")!!.source.contains("Hello again."))
        assertTrue(pc.items["script/$phoneId"]!!.contains("Edited on the phone."))

        // both edit the same script → PC keeps its id, the phone's version becomes a copy on both sides
        pc.items["script/pc-pitch"] = "---\ntitle: PC pitch\n---\n# A\nPC edit.\n"
        phone.putScriptSource("pc-pitch", "---\ntitle: PC pitch\n---\n# A\nPhone edit.\n")
        val c = SyncEngine(phone, pc, base).run()
        assertEquals(listOf("pc-pitch-phone"), c.copies)
        assertTrue(phone.get("pc-pitch")!!.source.contains("PC edit."))
        assertTrue(phone.get("pc-pitch-phone")!!.title.endsWith("(phone copy)"))
        assertTrue(pc.items["script/pc-pitch-phone"]!!.contains("Phone edit."))

        // delete on the phone → deleted on the PC; delete on the PC → deleted on the phone
        phone.deleteScript(phoneId)
        pc.items.remove("script/pc-pitch-phone"); pc.tombs.add("script/pc-pitch-phone")
        SyncEngine(phone, pc, base).run()
        assertNull(pc.items["script/$phoneId"])
        assertNull(phone.get("pc-pitch-phone"))
    }

    @Test
    fun rehearsalsMerge() {
        val phone = repo()
        val pc = FakePc()
        val base = HashMap<String, String>()
        phone.saveScript(null, "S", "Hi.", 140, 0.0)
        phone.addRun("s", kotlinx.serialization.json.JsonObject(mapOf("at" to kotlinx.serialization.json.JsonPrimitive("2026-11-21T10:00:00Z"))))
        pc.items["rehearsals/s"] = """[{"at":"2026-11-20T09:00:00Z","actual":60}]"""
        SyncEngine(phone, pc, base).run()
        val runs = phone.runs("s").map { it["at"].toString().trim('"') }
        assertEquals(listOf("2026-11-21T10:00:00Z", "2026-11-20T09:00:00Z"), runs)
        assertTrue(pc.items["rehearsals/s"]!!.contains("2026-11-21T10:00:00Z"))
    }

    /** Pairs with and syncs against the real Go hub (tools/hubtest). Set TC_HUB_BIN to run. */
    @Test
    fun interopWithGoHub() {
        val bin = System.getenv("TC_HUB_BIN")
        assumeTrue("TC_HUB_BIN not set", bin != null)
        val pcDir = Files.createTempDirectory("pc").toFile()
        File(pcDir, "scripts").mkdirs()
        File(pcDir, "scripts/from-pc.md").writeText("---\ntitle: From PC\nwpm: 150\n---\n# Opening\nkeywords: hello\nHello from the PC.\n")
        val proc = ProcessBuilder(bin, "-dir", pcDir.path).redirectErrorStream(true).start()
        try {
            val out = BufferedReader(InputStreamReader(proc.inputStream))
            fun next(prefix: String): String { while (true) { val l = out.readLine() ?: error("hub exited"); if (l.startsWith(prefix)) return l.removePrefix(prefix).trim() } }
            val port = next("READY ").toInt()
            // pair
            val pairing = HubClient("127.0.0.1", port, null)
            val start = pairing.pairBegin("phone-xyz", "JVM test phone")
            val code = pairing.pairNonce(start, ByteArray(32).also { SecureRandom().nextBytes(it) })
            assertEquals(6, code.length)
            val token = pairing.pairConfirm(start, true)
            assertNotNull(token)
            assertEquals(code, next("CODE ")) // the PC showed the same code
            // a wrong certificate pin is refused
            val wrong = HubClient("127.0.0.1", port, "00".repeat(32), "phone-xyz", token!!)
            assertTrue(runCatching { wrong.manifest() }.isFailure)
            // sync both ways
            val client = HubClient("127.0.0.1", port, pairing.seenFp, "phone-xyz", token)
            val phone = repo()
            val pid = phone.saveScript(null, "From phone", "# Close\nkeywords: thanks\nThanks from the phone.", 140, 90.0)
            phone.saveSettings(SyncedSettings(wpm = 165, fontSize = 50))
            val pk = phone.savePack(com.telecyborg.core.Pack(title = "Phone pack", topics = listOf("A", "B")))
            val base = HashMap<String, String>()
            val r = SyncEngine(phone, client.asRemote(), base).run()
            assertEquals(1, r.pulled)
            assertTrue(phone.get("from-pc")!!.source.contains("Hello from the PC."))
            assertTrue(File(pcDir, "scripts/$pid.md").readText().contains("Thanks from the phone."))
            assertTrue(File(pcDir, "settings.json").readText().contains("\"wpm\": 165"))
            assertTrue(File(pcDir, "packs/${pk.id}/pack.json").exists())
            assertTrue(File(pcDir, "packs/${pk.id}/claude-brief.md").readText().contains("Phone pack"))
            // second run: nothing left to do (hashes agree on both sides)
            val again = SyncEngine(phone, client.asRemote(), base).run()
            assertEquals("Up to date", again.toString())
            // delete on the phone reaches the PC
            phone.deleteScript(pid)
            SyncEngine(phone, client.asRemote(), base).run()
            assertTrue(!File(pcDir, "scripts/$pid.md").exists())
        } finally {
            proc.destroy()
        }
    }
}
