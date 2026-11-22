package com.telecyborg.core

import kotlinx.serialization.Serializable

/** Port of internal/pack (Go): content packs and the brief for Claude. Checked by GoldenTest. */
@Serializable
data class Pack(
    val id: String = "",
    val title: String = "",
    val occasion: String = "interview",
    val audience: String = "",
    val goal: String = "",
    val target: String = "2:00",
    val wpm: Int = 140,
    val lang: String = "en",
    val topics: List<String> = emptyList(),
    val facts: List<String> = emptyList(),
    val avoid: List<String> = emptyList(),
    val sources: String = "",
    val qaCount: Int = 8,
    val answerMax: Int = 60,
    val scriptId: String = "",
    val created: String = "",
    val modified: String = "",
)

data class Preview(
    val body: String,
    val title: String,
    val sections: Int,
    val qa: Int,
    val words: Int,
    val duration: Double,
    val target: Double,
    val checks: Int,
    val warnings: List<String>,
    val errors: List<String>,
    val ok: Boolean,
    val sectionTitles: List<String>,
)

object Packs {
    private val occasionWords = mapOf("interview" to "a job interview", "customer meeting" to "a customer meeting", "demo" to "a product demo", "other" to "a presentation")
    val langNames = mapOf("en" to "English", "es" to "Spanish", "pt" to "Portuguese")

    private fun clean(items: List<String>): List<String> = items.flatMap { it.split("\n") }
        .map { it.trim().trimStart('-', '*', '•', ' ') }.filter { it.isNotEmpty() }

    fun normalize(p0: Pack): Pack {
        var p = p0.copy(title = p0.title.trim().ifEmpty { "Untitled pack" })
        if (p.occasion !in occasionWords) p = p.copy(occasion = "interview")
        if (p.lang !in langNames) p = p.copy(lang = "en")
        p = p.copy(wpm = Script.clampWpm(if (p.wpm == 0) Script.DEFAULT_WPM else p.wpm))
        val t = if (Script.parseDuration(p.target) <= 0) "2:00" else p.target
        p = p.copy(target = Script.formatDuration(Script.parseDuration(t)))
        if (p.qaCount <= 0 || p.qaCount > 30) p = p.copy(qaCount = 8)
        if (p.answerMax < 20 || p.answerMax > 180) p = p.copy(answerMax = 60)
        return p.copy(topics = clean(p.topics), facts = clean(p.facts), avoid = clean(p.avoid))
    }

    private fun orDash(s: String) = s.trim().ifEmpty { "—" }

    fun markdown(p: Pack): String {
        val b = StringBuilder()
        b.append("# ${p.title}\n\n")
        b.append("- Occasion: ${p.occasion}\n- Audience: ${orDash(p.audience)}\n- Goal: ${orDash(p.goal)}\n- Pitch length: ${p.target} at ${p.wpm} wpm\n- Language: ${langNames[p.lang]}\n- Likely questions: ${p.qaCount} (answers up to ${p.answerMax} s)\n\n")
        fun list(title: String, items: List<String>) {
            if (items.isEmpty()) return
            b.append("## $title\n\n"); items.forEach { b.append("- $it\n") }; b.append("\n")
        }
        list("Topics", p.topics); list("Must-mention facts", p.facts); list("Avoid", p.avoid)
        if (p.sources.isNotBlank()) b.append("## Sources\n\n${p.sources.trim()}\n")
        return b.toString()
    }

    fun brief(p0: Pack): String {
        val p = normalize(p0)
        val target = Script.parseDuration(p.target)
        val totalWords = (target * p.wpm / 60 + 0.5).toInt()
        val sections = p.topics.size + 2
        val per = totalWords / sections
        val answerWords = p.answerMax * p.wpm / 60
        val lang = langNames[p.lang]
        val b = StringBuilder()
        b.append("You are helping me prepare for ${occasionWords[p.occasion]}. Write a spoken pitch I will read from a teleprompter, plus the questions I am likely to be asked with short spoken answers.\n\n")
        b.append("## Context\n\n")
        b.append("- Title: ${p.title}\n- Audience: ${orDash(p.audience)}\n- My goal: ${orDash(p.goal)}\n- Language: write everything in $lang.\n\n")
        b.append("## Timing\n\n")
        b.append("- The pitch must last ${p.target} at ${p.wpm} words per minute: about $totalWords words in total, not counting headings and keyword lines.\n")
        b.append("- Use $sections sections: an opening, one section per topic below, and a close. Aim for about $per words per section; shift words toward the topics that matter most to this audience.\n")
        b.append("- Write ${p.qaCount} likely questions. Each answer must fit in ${p.answerMax} seconds (at most $answerWords words).\n\n")
        if (p.topics.isNotEmpty()) {
            b.append("## Topics (in this order)\n\n")
            p.topics.forEachIndexed { i, t -> b.append("${i + 1}. $t\n") }
            b.append("\n")
        }
        if (p.facts.isNotEmpty()) {
            b.append("## Facts I must mention (use them exactly as written)\n\n"); p.facts.forEach { b.append("- $it\n") }; b.append("\n")
        }
        if (p.avoid.isNotEmpty()) {
            b.append("## Avoid\n\n"); p.avoid.forEach { b.append("- $it\n") }; b.append("\n")
        }
        if (p.sources.isNotBlank()) {
            b.append("## Sources (the only material you may draw facts from)\n\n").append(p.sources.trim()).append("\n\n")
        }
        val fence = "```"
        b.append("""## Rules for facts

- Use only the facts and sources above. Never invent numbers, names, dates, customers or results.
- If a sentence needs a fact you do not have, write the sentence with a placeholder and put [check] right after it, so I can verify or replace it.
- Write for the ear: short sentences, plain words, first person, natural pauses. No bullet points inside the text.

## Keywords

Under every heading, add one line that starts with `keywords:` followed by 3–8 comma-separated words or short phrases the OTHER person would say that should bring that section up (for example "pricing, cost, budget"). Include synonyms and, if the other person may speak another language, the same words in that language. Use a slash for alternatives of one idea: "price/cost".

## Output format

Reply with ONLY the Markdown below inside one code block, nothing before or after it. Keep the exact structure: the front matter, "# " headings, the keywords line under each heading, blank lines between paragraphs, the line "=== QA ===" before the questions, and "# Q: " headings for the questions. You may add "[pause 1]" on its own line where I should pause, "**word**" to stress a word, and "> note" lines as short stage directions for me (they are not read aloud).

${fence}markdown
---
title: ${p.title}
target: ${p.target}
wpm: ${p.wpm}
lang: ${p.lang}
---
# <Section title>
keywords: <comma-separated words the listener would say>
<paragraph>

<paragraph>

=== QA ===
# Q: <likely question>
keywords: <comma-separated words in the question>
<answer paragraphs>
$fence
""")
        return b.toString()
    }

    private val fenceRe = Regex("```(?:markdown|md)?[ \\t]*\\n(.*?)\\n```", RegexOption.DOT_MATCHES_ALL)

    fun extract(answer0: String): String {
        val answer = answer0.replace("\r\n", "\n")
        fenceRe.find(answer)?.let { return it.groupValues[1].trim() + "\n" }
        var t = answer.trim()
        val i = t.indexOf("---\n")
        if (i in 1..399) t = t.substring(i)
        return t + "\n"
    }

    private fun quote(s: String) = "\"" + s.replace("\\", "\\\\").replace("\"", "\\\"") + "\""

    fun validate(answer: String, p0: Pack): Preview {
        val p = normalize(p0)
        val body = extract(answer)
        val ps = Script.parse(body)
        val warnings = ArrayList<String>()
        val errors = ArrayList<String>()
        var wpm = ps.meta.wpm
        var target = ps.meta.target
        var title = ps.meta.title
        val fm = Script.splitFrontMatter(body).first
        if ("wpm" !in fm) wpm = p.wpm
        if ("target" !in fm) target = Script.parseDuration(p.target)
        if ("title" !in fm) title = p.title
        val duration = ps.duration(wpm)
        val sections = ps.sections - ps.qaItems
        if (ps.paragraphs == 0) errors.add("No pitch text found. Paste Claude's whole reply, including the code block.")
        if (sections == 0) errors.add("No sections found: headings must start with \"# \".")
        if (ps.qaItems == 0) warnings.add("No questions found (\"=== QA ===\" and \"# Q:\" headings). The pitch will import without cue answers.")
        else if (ps.qaItems < p.qaCount / 2) warnings.add("Only ${ps.qaItems} questions; the pack asked for ${p.qaCount}.")
        val noKw = ArrayList<String>()
        for (o in ps.outline) {
            if (o.keywords.isEmpty()) noKw.add(o.title)
            if (o.qa && o.paras == 0) warnings.add("${quote(o.title)} has no answer text.")
            val secs = o.words * 60.0 / wpm
            if (o.qa && secs > p.answerMax * 1.25) warnings.add("${quote(o.title)} runs about ${Script.formatDuration(secs)}, longer than ${p.answerMax} s.")
        }
        if (noKw.isNotEmpty()) {
            val shown = if (noKw.size > 4) noKw.take(4) + "…" else noKw
            warnings.add("${noKw.size} section(s) have no keywords line; they are matched by title only: ${shown.joinToString("; ")}.")
        }
        if (target > 0 && duration > target * 1.15) warnings.add("The pitch reads in ${Script.formatDuration(duration)} at $wpm wpm, over the ${Script.formatDuration(target)} target. Fit to time will raise the pace, or cut text.")
        if (ps.checks > 0) warnings.add("${ps.checks} [check] mark(s): facts Claude could not confirm. Verify them in the editor before you present.")
        return Preview(body, title, sections, ps.qaItems, ps.words, duration, target, ps.checks, warnings, errors, errors.isEmpty(), ps.outline.map { it.title })
    }

    fun withPack(body: String, p: Pack): String {
        val (fm, rest) = Script.splitFrontMatter(body)
        if (fm["title"].isNullOrEmpty()) fm["title"] = p.title
        if (fm["target"].isNullOrEmpty()) fm["target"] = p.target
        if (fm["wpm"].isNullOrEmpty()) fm["wpm"] = p.wpm.toString()
        if (fm["lang"].isNullOrEmpty()) fm["lang"] = p.lang
        fm["pack"] = p.id
        val keys = listOf("title", "target", "wpm", "lang", "pack")
        val extra = fm.keys.filter { it !in keys }.sorted()
        val b = StringBuilder("---\n")
        for (k in keys + extra) b.append(k).append(": ").append((fm[k] ?: "").replace("\n", " ")).append("\n")
        b.append("---\n").append(rest.trimStart('\n'))
        return b.toString()
    }
}

object Hash {
    /** Hex SHA-256 of the content: the sync hash both PC and phone compute. */
    fun of(content: String): String {
        val d = java.security.MessageDigest.getInstance("SHA-256").digest(content.toByteArray(Charsets.UTF_8))
        return d.joinToString("") { "%02x".format(it) }
    }
}
