package com.telecyborg.core

/**
 * Port of internal/script (Go): TeleCyborg's Markdown script format
 * (docs/requirements/04-script-format.md). Checked against the Go output by GoldenTest.
 */
data class Meta(
    val title: String = "",
    val wpm: Int = Script.DEFAULT_WPM,
    val target: Double = 0.0,
    val fontSize: Int = 0,
    val lang: String = "",
    val extra: Map<String, String> = emptyMap(),
)

/** One unit of the prompter. [lines] keeps the raw lines for styled display (bold, aside). */
data class Block(
    val kind: String, // section, cue, para
    val text: String,
    val lines: List<String> = emptyList(),
    val words: Int = 0,
    var pause: Double = 0.0,
    val index: Int = -1,
    val section: String = "",
    val sec: Int = -1,
    val qa: Boolean = false,
)

data class Section(
    val index: Int,
    val title: String,
    val qa: Boolean,
    val keywords: MutableList<String> = mutableListOf(),
    var firstPara: Int = -1,
    var paras: Int = 0,
    var words: Int = 0,
)

data class Parsed(
    val meta: Meta,
    val blocks: List<Block>,
    val words: Int,
    val paragraphs: Int,
    val sections: Int,
    val pauses: Double,
    val outline: List<Section>,
    val qaItems: Int,
    val qaWords: Int,
    val qaParas: Int,
    val checks: Int,
) {
    fun duration(wpm: Int = meta.wpm): Double = words * 60.0 / (if (wpm <= 0) Script.DEFAULT_WPM else wpm) + pauses

    fun fitWpm(target: Double): Int {
        val avail = target - pauses
        if (avail <= 0 || words == 0) return Script.MAX_WPM
        return Script.clampWpm((words * 60.0 / avail + 0.5).toInt())
    }
}

object Script {
    const val DEFAULT_WPM = 140
    const val MIN_WPM = 60
    const val MAX_WPM = 300
    const val DEFAULT_PAUSE = 1.5

    private val pauseRe = Regex("""^\[pause(?:\s+([0-9]+(?:\.[0-9]+)?)\s*s?)?\]$""", RegexOption.IGNORE_CASE)
    private val headingRe = Regex("""^(#{1,3})\s+(.+?)\s*#*$""")
    private val commentRe = Regex("""<!--.*?-->""", RegexOption.DOT_MATCHES_ALL)
    private val boldRe = Regex("""\*\*(.+?)\*\*""")
    private val asideRe = Regex("""(^|[\s(])_(.+?)_([\s.,;:!?)]|$)""")
    private val wordRe = Regex("""\S+""")
    private val qaMarkRe = Regex("""^={3,}\s*(?:QA|Q&A|Q & A)\s*={3,}$""", RegexOption.IGNORE_CASE)
    val checkRe = Regex("""\[check\]""", RegexOption.IGNORE_CASE)

    fun clampWpm(n: Int) = n.coerceIn(MIN_WPM, MAX_WPM)

    fun isQATitle(t0: String): Boolean {
        val t = t0.trim()
        return t.length > 2 && (t.startsWith("Q:") || t.startsWith("q:") || t.startsWith("P:") || t.startsWith("Q."))
    }

    private fun hasLetterOrDigit(w: String): Boolean {
        var i = 0
        while (i < w.length) {
            val cp = w.codePointAt(i)
            if (Character.isLetter(cp) || Character.isDigit(cp)) return true
            i += Character.charCount(cp)
        }
        return false
    }

    fun words(s: String): List<String> = wordRe.findAll(s).map { it.value }.filter(::hasLetterOrDigit).toList()

    fun countWords(s: String) = words(s).size

    fun splitKeywords(v: String): List<String> {
        val seen = HashSet<String>()
        val out = ArrayList<String>()
        for (k0 in v.split(',', ';')) {
            val k = k0.trim()
            if (k.isEmpty() || !seen.add(k.lowercase())) continue
            out.add(k)
        }
        return out
    }

    /** Front matter ("---" … "---") and the body after it. */
    fun splitFrontMatter(src0: String): Pair<LinkedHashMap<String, String>, String> {
        val src = src0.replace("\r\n", "\n").removePrefix("﻿")
        val fm = LinkedHashMap<String, String>()
        if (!src.startsWith("---\n")) return fm to src
        val rest = src.substring(4)
        val end = rest.indexOf("\n---")
        if (end < 0) return fm to src
        for (line in rest.substring(0, end).split("\n")) {
            val i = line.indexOf(':')
            if (i < 0) continue
            fm[line.substring(0, i).trim()] = line.substring(i + 1).trim().trim('"', '\'')
        }
        var body = rest.substring(end + 4)
        val nl = body.indexOf('\n')
        body = if (nl >= 0) body.substring(nl + 1) else ""
        return fm to body
    }

    /** "m:ss", "h:mm:ss", "90", "90s" → seconds (0 if empty or invalid). */
    fun parseDuration(s0: String): Double {
        val s = s0.trim().removeSuffix("s").trim()
        if (s.isEmpty()) return 0.0
        var total = 0.0
        for (p in s.split(":")) {
            val v = p.trim().toDoubleOrNull() ?: return 0.0
            if (v < 0) return 0.0
            total = total * 60 + v
        }
        return total
    }

    fun formatDuration(sec: Double): String {
        if (sec <= 0) return ""
        val t = (sec + 0.5).toInt()
        return "${t / 60}:${(t % 60).toString().padStart(2, '0')}"
    }

    private fun metaFrom(fm: Map<String, String>): Meta {
        var m = Meta()
        val extra = LinkedHashMap<String, String>()
        for ((k, v) in fm) {
            when (k.lowercase()) {
                "title" -> m = m.copy(title = v)
                "wpm" -> v.toIntOrNull()?.let { m = m.copy(wpm = clampWpm(it)) }
                "target" -> m = m.copy(target = parseDuration(v))
                "fontsize" -> v.toIntOrNull()?.let { if (it in 16..120) m = m.copy(fontSize = it) }
                "lang" -> m = m.copy(lang = v)
                else -> extra[k] = v
            }
        }
        return m.copy(extra = extra)
    }

    /** Text without markup, as counted and searched. */
    fun plain(s0: String): String {
        var s = boldRe.replace(s0, "$1")
        s = asideRe.replace(s, "$1$2$3")
        s = checkRe.replace(s, "")
        return s.trim().split(Regex("""\s+""")).filter { it.isNotEmpty() }.joinToString(" ")
    }

    fun parse(src: String): Parsed {
        val (fm, body0) = splitFrontMatter(src)
        var meta = metaFrom(fm)
        val body = commentRe.replace(body0, "")
        val blocks = ArrayList<Block>()
        val outline = ArrayList<Section>()
        var words = 0; var paragraphs = 0; var sections = 0; var pauses = 0.0
        var qaItems = 0; var qaWords = 0; var qaParas = 0
        var section = ""
        var sec = -1
        var inQA = false
        val para = ArrayList<String>()

        fun flush() {
            if (para.isEmpty()) return
            val plains = para.map(::plain).filter { it.isNotEmpty() }
            val text = plains.joinToString("\n")
            val qa = inQA || (sec >= 0 && outline[sec].qa)
            val n = countWords(text)
            val index: Int
            if (qa) { index = qaParas; qaParas++; qaWords += n } else { index = paragraphs; words += n; paragraphs++ }
            if (sec >= 0) {
                val o = outline[sec]
                if (o.firstPara < 0) o.firstPara = index
                o.paras++; o.words += n
            }
            blocks.add(Block("para", text, para.toList(), n, 0.0, index, section, sec, qa))
            para.clear()
        }

        for (raw in body.split("\n")) {
            val line = raw.trim()
            when {
                line.isEmpty() -> flush()
                qaMarkRe.matches(line) -> { flush(); inQA = true }
                pauseRe.matches(line) -> {
                    flush()
                    val m = pauseRe.find(line)!!
                    val d = m.groupValues[1].takeIf { it.isNotEmpty() }?.toDouble() ?: DEFAULT_PAUSE
                    val lp = blocks.lastOrNull { it.kind == "para" }
                    if (lp != null) {
                        lp.pause += d
                        if (!lp.qa) pauses += d
                    }
                }
                headingRe.matches(line) -> {
                    flush()
                    section = headingRe.find(line)!!.groupValues[2].trim()
                    val qa = inQA || isQATitle(section)
                    sec = outline.size
                    outline.add(Section(sec, section, qa))
                    if (qa) qaItems++
                    blocks.add(Block("section", section, listOf(section), 0, 0.0, -1, section, sec, qa))
                    sections++
                }
                line.startsWith(">") -> {
                    flush()
                    val cue = line.trimStart('>').trim()
                    if (cue.isNotEmpty()) blocks.add(Block("cue", cue, listOf(cue), 0, 0.0, -1, section, sec, inQA || (sec >= 0 && outline[sec].qa)))
                }
                line.lowercase().startsWith("keywords:") -> {
                    flush()
                    if (sec >= 0) outline[sec].keywords.addAll(splitKeywords(line.substring("keywords:".length)))
                }
                else -> para.add(line)
            }
        }
        flush()
        val checks = checkRe.findAll(body).count()
        var p = Parsed(meta, blocks, words, paragraphs, sections, pauses, outline, qaItems, qaWords, qaParas, checks)
        if (meta.title.isEmpty()) {
            meta = meta.copy(title = guessTitle(p))
            p = p.copy(meta = meta)
        }
        return p
    }

    private fun guessTitle(p: Parsed): String {
        p.blocks.firstOrNull { it.kind == "section" && !it.qa }?.let { return it.text }
        p.blocks.firstOrNull { it.kind == "para" }?.let { b ->
            val w = b.text.split(Regex("""\s+""")).filter { it.isNotEmpty() }.take(6)
            return w.joinToString(" ") + "…"
        }
        return "Untitled script"
    }

    /** Builds a script file from metadata and a body (front matter regenerated), like Go's Compose. */
    fun compose(m: Meta, body0: String): String {
        val body = splitFrontMatter(body0).second
        val b = StringBuilder("---\n")
        if (m.title.isNotEmpty()) b.append("title: ").append(m.title.replace("\n", " ")).append("\n")
        if (m.wpm > 0) b.append("wpm: ").append(clampWpm(m.wpm)).append("\n")
        if (m.target > 0) b.append("target: ").append(formatDuration(m.target)).append("\n")
        if (m.fontSize > 0) b.append("fontSize: ").append(m.fontSize).append("\n")
        if (m.lang.isNotEmpty()) b.append("lang: ").append(m.lang).append("\n")
        for ((k, v) in m.extra) b.append(k).append(": ").append(v).append("\n")
        b.append("---\n").append(body.trimStart('\n'))
        if (!body.endsWith("\n")) b.append("\n")
        return b.toString()
    }

    /** File-safe id from a title (same rules as Go's store.Slug). */
    fun slug(title: String): String {
        val t = java.text.Normalizer.normalize(title.lowercase(), java.text.Normalizer.Form.NFD)
        val b = StringBuilder()
        var dash = false
        var i = 0
        while (i < t.length) {
            val cp = t.codePointAt(i)
            i += Character.charCount(cp)
            when {
                Character.getType(cp) == Character.NON_SPACING_MARK.toInt() -> continue
                cp < 128 && (Character.isLetter(cp) || Character.isDigit(cp)) -> { b.appendCodePoint(cp); dash = false }
                else -> if (!dash && b.isNotEmpty()) { b.append('-'); dash = true }
            }
        }
        var out = b.toString().trim('-')
        if (out.length > 60) out = out.substring(0, 60).trim('-')
        return out.ifEmpty { "script" }
    }

    private val idRe = Regex("""^[a-z0-9][a-z0-9-]{0,79}$""")
    fun validId(id: String) = idRe.matches(id)
}
