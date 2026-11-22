package com.telecyborg

import com.telecyborg.core.Hash
import com.telecyborg.core.Pack
import com.telecyborg.core.Packs
import com.telecyborg.core.Script
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.boolean
import kotlinx.serialization.json.double
import kotlinx.serialization.json.int
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Test
import java.io.File

/** The Kotlin port must agree with the Go code (fixtures in android/golden, written by Go). */
class GoldenTest {
    private val dir = File("../golden")
    private fun json(name: String) = Json.parseToJsonElement(File(dir, name).readText()).jsonObject
    private fun JsonObject.s(k: String) = this[k]?.jsonPrimitive?.content ?: ""
    private fun JsonObject.i(k: String) = this[k]?.jsonPrimitive?.int ?: 0
    private fun JsonObject.d(k: String) = this[k]?.jsonPrimitive?.double ?: 0.0
    private fun JsonObject.b(k: String) = this[k]?.jsonPrimitive?.boolean ?: false

    @Test
    fun parserMatchesGo() {
        for (n in listOf("pitch", "plain")) {
            val g = json("$n.parsed.json")
            val want = g["parsed"]!!.jsonObject
            val p = Script.parse(File(dir, "$n.md").readText())
            val meta = want["meta"]!!.jsonObject
            assertEquals("$n title", meta.s("title"), p.meta.title)
            assertEquals(meta.i("wpm"), p.meta.wpm)
            assertEquals(meta.d("target"), p.meta.target, 1e-9)
            assertEquals(meta.s("lang"), p.meta.lang)
            val extra = meta["extra"]?.jsonObject?.mapValues { it.value.jsonPrimitive.content } ?: emptyMap()
            assertEquals(extra, p.meta.extra)
            for (k in listOf("words", "paragraphs", "sections", "qaItems", "qaWords", "qaParas", "checks")) {
                val got = mapOf("words" to p.words, "paragraphs" to p.paragraphs, "sections" to p.sections, "qaItems" to p.qaItems,
                    "qaWords" to p.qaWords, "qaParas" to p.qaParas, "checks" to p.checks)[k]
                assertEquals("$n $k", want.i(k), got)
            }
            assertEquals(want.d("pauses"), p.pauses, 1e-9)
            val wb = want["blocks"]!!.jsonArray
            assertEquals("$n blocks", wb.size, p.blocks.size)
            wb.forEachIndexed { i, e ->
                val b = e.jsonObject
                val k = p.blocks[i]
                assertEquals("$n block $i kind", b.s("kind"), k.kind)
                assertEquals("$n block $i text", b.s("text"), k.text)
                assertEquals("$n block $i words", b.i("words"), k.words)
                assertEquals("$n block $i pause", b.d("pause"), k.pause, 1e-9)
                assertEquals("$n block $i index", b.i("index"), k.index)
                assertEquals("$n block $i sec", b.i("sec"), k.sec)
                assertEquals("$n block $i qa", b.b("qa"), k.qa)
            }
            val wo = (want["outline"] as JsonArray)
            assertEquals(wo.size, p.outline.size)
            wo.forEachIndexed { i, e ->
                val o = e.jsonObject
                val k = p.outline[i]
                assertEquals(o.s("title"), k.title)
                assertEquals(o.b("qa"), k.qa)
                assertEquals(o["keywords"]!!.jsonArray.map { it.jsonPrimitive.content }, k.keywords)
                assertEquals(o.i("firstPara"), k.firstPara)
                assertEquals(o.i("paras"), k.paras)
                assertEquals(o.i("words"), k.words)
            }
            assertEquals(g.d("duration"), p.duration(), 1e-9)
            assertEquals(g.i("fitWpm90"), p.fitWpm(90.0))
        }
    }

    @Test
    fun briefAndValidateMatchGo() {
        val pack = Json { ignoreUnknownKeys = true }.decodeFromString(Pack.serializer(), File(dir, "pack.json").readText()).copy(id = "terrific-interview")
        assertEquals(File(dir, "pack.brief.txt").readText(), Packs.brief(pack))
        val reply = "Here it is:\n\n```markdown\n" + File(dir, "pitch.md").readText() + "```\nGood luck"
        val g = json("pack.validate.json")
        val want = g["preview"]!!.jsonObject
        val pv = Packs.validate(reply, pack)
        assertEquals(want.s("body"), pv.body.trim())
        assertEquals(want.s("title"), pv.title)
        assertEquals(want.i("sections"), pv.sections)
        assertEquals(want.i("qa"), pv.qa)
        assertEquals(want.i("words"), pv.words)
        assertEquals(want.d("duration"), pv.duration, 1e-9)
        assertEquals(want.d("target"), pv.target, 1e-9)
        assertEquals(want.i("checks"), pv.checks)
        assertEquals(want["warnings"]!!.jsonArray.map { it.jsonPrimitive.content }, pv.warnings)
        assertEquals(want["errors"]!!.jsonArray.map { it.jsonPrimitive.content }, pv.errors)
        assertEquals(g.s("withPack"), Packs.withPack(Packs.extract(reply), pack))
    }

    @Test
    fun hashMatchesGo() {
        assertEquals(File(dir, "pitch.hash.txt").readText().trim(), Hash.of(File(dir, "pitch.md").readText()))
    }

    @Test
    fun slugAndDurations() {
        assertEquals("pitch-nandu", Script.slug("Pitch ñandú!"))
        assertEquals(90.0, Script.parseDuration("1:30"), 1e-9)
        assertEquals("1:30", Script.formatDuration(90.0))
    }
}
