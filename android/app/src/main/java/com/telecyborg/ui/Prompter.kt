package com.telecyborg.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.gestures.detectTapGestures
import androidx.compose.foundation.gestures.detectVerticalDragGestures
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawingPadding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateMapOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.runtime.withFrameNanos
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.layout.onGloballyPositioned
import androidx.compose.ui.layout.positionInParent
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.LocalView
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.font.FontStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.withStyle
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.telecyborg.core.Block
import com.telecyborg.core.Script
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlin.math.abs
import kotlin.math.max
import kotlin.math.min
import kotlin.math.roundToInt

/** A paragraph on the plan clock (docs/requirements/03-pacing-and-timing.md). */
class Para(val block: Block, val words: Int, val read: Double, val pause: Double, val start: Double) {
    val plan get() = read + pause
    var actual = 0.0
    var skipped = false
}

/** A track: the narrative, or one Q&A answer (a detour with its own clock). */
class Track(val blocks: List<Block>, wpm: Int, val sec: Int, val title: String) {
    val paras: List<Para>
    val total: Double
    var s = 0.0
    var time = 0.0

    init {
        var t = 0.0
        paras = blocks.filter { it.kind == "para" }.map { b ->
            val read = max(0.6, b.words * 60.0 / wpm)
            Para(b, b.words, read, b.pause, t).also { t += read + b.pause }
        }
        total = t
    }

    fun paraAt(s: Double): Int {
        var lo = 0; var hi = paras.size - 1
        while (lo < hi) { val mid = (lo + hi + 1) / 2; if (paras[mid].start <= s) lo = mid else hi = mid - 1 }
        return max(0, lo)
    }
}

data class RunResult(val run: JsonObject, val texts: List<String>)

/** Styled text: **stress**, _aside_, [check]. */
fun styled(lines: List<String>): AnnotatedString = buildAnnotatedString {
    lines.forEachIndexed { li, line ->
        if (li > 0) append("\n")
        var i = 0
        val re = Regex("""\*\*(.+?)\*\*|(^|(?<=[\s(]))_(.+?)_(?=[\s.,;:!?)]|$)|\[check\]""", RegexOption.IGNORE_CASE)
        for (m in re.findAll(line)) {
            append(line.substring(i, m.range.first))
            when {
                m.value.startsWith("**") -> withStyle(SpanStyle(fontWeight = FontWeight.Bold, color = Color.White)) { append(m.groupValues[1]) }
                m.value.equals("[check]", true) -> withStyle(SpanStyle(color = WarnC, fontSize = 14.sp)) { append(" [check]") }
                else -> withStyle(SpanStyle(fontStyle = FontStyle.Italic, color = Ghost)) { append(m.groupValues[3]) }
            }
            i = m.range.last + 1
        }
        append(line.substring(i))
    }
}

@Composable
fun PrompterScreen(m: AppModel, id: String) {
    val doc = remember(id) { m.repo.get(id) } ?: run { m.back(); return }
    val view = LocalView.current
    DisposableEffect(Unit) { view.keepScreenOn = true; onDispose { view.keepScreenOn = false } }
    val st = m.settings
    val p = doc.parsed
    val planWpm = doc.wpm
    val main = remember(id) { Track(p.blocks.filter { !it.qa }, planWpm, -1, doc.title) }
    var detour by remember(id) { mutableStateOf<Track?>(null) }
    var returnTo by remember(id) { mutableStateOf(0.0) }
    var live by remember(id) { mutableIntStateOf(planWpm) }
    var playing by remember(id) { mutableStateOf(false) }
    var count by remember(id) { mutableIntStateOf(st.countdown) }
    var elapsed by remember(id) { mutableStateOf(0.0) }
    var wall by remember(id) { mutableStateOf(0.0) }
    var font by remember(id) { mutableIntStateOf(st.fontSize) }
    var mirror by remember(id) { mutableStateOf(st.mirror) }
    var showQA by remember(id) { mutableStateOf(false) }
    var tick by remember(id) { mutableIntStateOf(0) } // forces recomposition of the HUD
    val answers = remember(id) { mutableListOf<JsonObject>() }
    var reached by remember(id) { mutableIntStateOf(0) }
    var done by remember(id) { mutableStateOf(false) }
    val target = if (doc.target > 0) doc.target else main.total
    val tr = detour ?: main

    fun recordDetour() { detour?.let { d -> if (d.time > 0.5) answers.add(JsonObject(mapOf("title" to JsonPrimitive(d.title), "planned" to JsonPrimitive(d.total), "actual" to JsonPrimitive(d.time)))) } }

    fun finish(completed: Boolean) {
        if (done) return
        done = true; playing = false
        recordDetour()
        if (elapsed < 2) { m.back(); return }
        val paras = main.paras.mapIndexed { k, q ->
            JsonObject(buildMap {
                put("planned", JsonPrimitive(q.plan)); put("actual", JsonPrimitive(q.actual))
                if (q.skipped) put("skipped", JsonPrimitive(true))
                if (!completed && k > reached && !q.skipped) put("notReached", JsonPrimitive(true))
            })
        }
        val run = JsonObject(buildMap {
            put("planned", JsonPrimitive(main.total)); put("actual", JsonPrimitive(elapsed)); put("target", JsonPrimitive(doc.target))
            put("wpmStart", JsonPrimitive(planWpm)); put("wpmEnd", JsonPrimitive(live)); put("completed", JsonPrimitive(completed))
            put("paras", JsonArray(paras)); put("mode", JsonPrimitive("clock")); put("wall", JsonPrimitive(wall))
            if (answers.isNotEmpty()) put("answers", JsonArray(answers.toList()))
            put("device", JsonPrimitive("phone"))
        })
        m.repo.addRun(id, run)
        m.changed()
        m.replace(Screen.Report(id, RunResult(run, main.paras.map { it.block.text })))
    }

    fun jump(delta: Int) {
        val t = tr
        val i = t.paraAt(t.s)
        var j = (i + delta).coerceIn(0, t.paras.size - 1)
        if (delta == -1 && t.s - t.paras[i].start > 1.5) j = i
        if (t === main) {
            for (k in i + 1 until j) if (main.paras[k].actual < 0.2) main.paras[k].skipped = true
            if (j > i && main.paras[i].actual < 0.2) main.paras[i].skipped = true
            main.paras[j].skipped = false
        }
        t.s = t.paras.getOrNull(j)?.start ?: 0.0
        tick++
    }

    fun openAnswer(sec: Int) {
        val o = p.outline[sec]
        val blocks = p.blocks.filter { it.sec == sec }
        if (blocks.none { it.kind == "para" }) return
        if (detour == null) returnTo = main.paras.getOrNull(main.paraAt(main.s))?.start ?: 0.0 else recordDetour()
        detour = Track(blocks, live, sec, o.title)
        showQA = false; playing = true; count = 0
    }

    fun back() { recordDetour(); detour = null; main.s = returnTo; tick++ }

    // countdown, then the clock
    LaunchedEffect(id) {
        while (count > 0) { kotlinx.coroutines.delay(1000); count-- }
        playing = true
        var last = 0L
        while (!done) {
            withFrameNanos { now ->
                val dt = if (last == 0L) 0.0 else min(0.1, (now - last) / 1e9)
                last = now
                val t = detour ?: main
                wall += dt
                if (t === main) elapsed += dt else t.time += dt
                val i = t.paraAt(t.s)
                if (t.paras.isNotEmpty()) {
                    t.paras[i].actual += dt
                    if (t === main && i > reached) reached = i
                    if (playing) {
                        val q = t.paras[i]
                        t.s += if (t.s - q.start >= q.read) dt else dt * live / planWpm
                        if (t === main && t.s >= t.total) { t.s = t.total; finish(true) }
                        if (t.s > t.total) t.s = t.total
                    }
                }
                tick++
            }
        }
    }

    // coaching (same formulas as the PC)
    val mi = main.paraAt(main.s)
    var remWords = 0.0; var remPause = 0.0
    main.paras.getOrNull(mi)?.let { q ->
        val into = main.s - q.start
        remWords = q.words * max(0.0, 1 - into / q.read)
        remPause = if (into >= q.read) max(0.0, q.plan - into) else q.pause
        for (k in mi + 1 until main.paras.size) { remWords += main.paras[k].words; remPause += main.paras[k].pause }
    }
    val deficit = elapsed - main.s
    val avail = target - elapsed - remPause
    val needed = if (avail > 0) remWords * 60 / avail else Double.POSITIVE_INFINITY
    val alert = when {
        detour != null -> if (detour!!.time > st.answerMax) "wrap" else ""
        deficit > 3 && (needed > 300 || !needed.isFinite()) -> "miss"
        deficit > 3 && needed.roundToInt() > live -> "behind"
        deficit < -5 && remWords > 0 -> "ahead"
        else -> ""
    }
    tick.let { }

    BoxWithConstraints(Modifier.fillMaxSize().background(Void)) {
        val density = LocalDensity.current
        val focusPx = with(density) { (maxHeight * st.focusLine.toFloat().coerceIn(0.15f, 0.6f)).toPx() }
        val tops = remember(tr) { mutableStateMapOf<Int, Pair<Int, Int>>() }
        val scroll = rememberScrollState()
        LaunchedEffect(tr) {
            while (true) {
                withFrameNanos {
                    val i = tr.paraAt(tr.s)
                    val q = tr.paras.getOrNull(i) ?: return@withFrameNanos
                    val (top, h) = tops[i] ?: return@withFrameNanos
                    val frac = ((tr.s - q.start) / q.read).coerceIn(0.0, 1.0)
                    val want = (top + frac * h - focusPx).toInt().coerceAtLeast(0)
                    val d = want - scroll.value
                    if (d != 0) scroll.dispatchRawDelta(if (abs(d) > 300) d.toFloat() else d * 0.5f)
                }
            }
        }
        val size = (font * 0.68f).coerceIn(16f, 80f).sp
        val tailH = maxHeight
        Box(Modifier.fillMaxSize()
            .pointerInput(tr) { detectTapGestures(onTap = { if (count > 0) count = 0 else playing = !playing }) }
            .pointerInput(tr) {
                var acc = 0f
                detectVerticalDragGestures(onDragEnd = { if (abs(acc) > 60) jump(if (acc < 0) 1 else -1); acc = 0f }) { _, d -> acc += d }
            }) {
            Column(Modifier.fillMaxSize().verticalScroll(scroll, enabled = false).padding(horizontal = 22.dp)
                .graphicsLayer { if (mirror) scaleX = -1f }, verticalArrangement = Arrangement.spacedBy(16.dp)) {
                Box(Modifier.height(with(density) { focusPx.toDp() }))
                var pi = 0
                val cur = tr.paraAt(tr.s)
                tr.blocks.forEach { b ->
                    when (b.kind) {
                        "section" -> Text(b.text.uppercase(), color = if (b.qa) WarnC else Code, fontFamily = Mono, fontSize = 12.sp, letterSpacing = 2.sp)
                        "cue" -> Text("› " + b.text, color = WarnC, fontSize = (size.value * 0.5f).sp)
                        else -> {
                            val k = pi++
                            Text(styled(b.lines), fontSize = size, lineHeight = (size.value * 1.4f).sp,
                                color = when { k < cur -> Ghost; k == cur -> ReadC; else -> ReadC.copy(alpha = 0.78f) },
                                modifier = Modifier.fillMaxWidth().onGloballyPositioned { tops[k] = it.positionInParent().y.toInt() to it.size.height })
                        }
                    }
                }
                Text(if (detour != null) "■ end of answer · tap ‹ back" else "■ end", color = Code, fontFamily = Mono)
                Box(Modifier.height(tailH))
            }
            // focus band
            Box(Modifier.offset(y = with(density) { focusPx.toDp() } - 4.dp).fillMaxWidth().height((size.value * 1.5f).dp)
                .border(1.dp, Code.copy(alpha = 0.35f)).background(Code.copy(alpha = 0.05f)))
            if (count > 0) Box(Modifier.fillMaxSize().background(Void.copy(alpha = 0.7f)), contentAlignment = Alignment.Center) {
                Text("$count", color = Code, fontSize = 120.sp, fontFamily = Mono)
            }
        }

        // top: answer bar / alert
        Column(Modifier.align(Alignment.TopCenter).safeDrawingPadding().padding(8.dp), horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(6.dp)) {
            detour?.let { d ->
                Row(Modifier.border(1.dp, if (alert == "wrap") Miss else WarnC).background(Color(0xEE140C00)).padding(horizontal = 12.dp, vertical = 6.dp),
                    horizontalArrangement = Arrangement.spacedBy(12.dp), verticalAlignment = Alignment.CenterVertically) {
                    Text(d.title, color = ReadC, fontSize = 13.sp, maxLines = 1, modifier = Modifier.widthIn(max = 360.dp))
                    Text("${fmt(d.time)} / ${fmt(d.total)}", color = WarnC, fontFamily = Mono, fontSize = 13.sp)
                    if (alert == "wrap") Text("wrap up", color = Miss, fontSize = 13.sp, fontWeight = FontWeight.Bold)
                    Text("‹ back", color = Code, fontSize = 14.sp, modifier = Modifier.clickable { back() })
                }
            }
            if (detour == null && alert.isNotEmpty()) {
                val (txt, col) = when (alert) {
                    "behind" -> "Behind by ${fmt(deficit)} — speed up to ${needed.roundToInt()} wpm (tap to apply)" to WarnC
                    "miss" -> "Behind by ${fmt(deficit)} — can't finish on time: swipe up to skip" to Miss
                    else -> "Ahead by ${fmt(-deficit)} — slow down a little" to Code
                }
                Text(txt, color = col, fontSize = 13.sp, modifier = Modifier.border(1.dp, col).background(Color(0xEE000500)).padding(horizontal = 10.dp, vertical = 6.dp)
                    .clickable { if (alert == "behind" && needed.isFinite()) live = Script.clampWpm(kotlin.math.ceil(needed).toInt()) })
            }
        }

        // Q&A list
        if (showQA) {
            Column(Modifier.align(Alignment.Center).padding(24.dp).border(1.dp, WarnC).background(PanelC).padding(12.dp).verticalScroll(rememberScrollState()),
                verticalArrangement = Arrangement.spacedBy(6.dp)) {
                Text("Answers", color = WarnC, fontFamily = Mono)
                p.outline.filter { it.qa && it.paras > 0 }.forEach { o ->
                    Text(o.title, color = ReadC, modifier = Modifier.fillMaxWidth().clickable { openAnswer(o.index) }.padding(vertical = 6.dp))
                }
                Text("Close", color = Ghost, modifier = Modifier.clickable { showQA = false }.padding(top = 6.dp))
            }
        }

        // HUD
        Column(Modifier.align(Alignment.BottomCenter).fillMaxWidth().background(PanelC).safeDrawingPadding()) {
            Row(Modifier.fillMaxWidth().padding(horizontal = 10.dp, vertical = 4.dp), horizontalArrangement = Arrangement.spacedBy(14.dp), verticalAlignment = Alignment.CenterVertically) {
                Text(if (playing) "❚❚" else "▶", color = Code, fontSize = 18.sp, modifier = Modifier.clickable { playing = !playing; if (count > 0) count = 0 })
                Text("$live wpm", color = ReadC, fontFamily = Mono)
                Text("${fmt(elapsed)} / ${fmt(target)}", color = if (elapsed + (main.total - main.s) * planWpm / live > target + 1) WarnC else ReadC, fontFamily = Mono)
                Text("¶ ${tr.paraAt(tr.s) + 1}/${tr.paras.size}", color = Ghost, fontFamily = Mono)
            }
            Row(Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()).padding(horizontal = 6.dp, vertical = 2.dp), horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                Btn("−5") { live = Script.clampWpm(live - 5) }
                Btn("+5") { live = Script.clampWpm(live + 5) }
                Btn("◀") { jump(-1) }
                Btn("▶") { jump(1) }
                Btn("A−") { font = max(24, font - 4) }
                Btn("A+") { font = min(110, font + 4) }
                Btn(if (mirror) "Mirror ✓" else "Mirror") { mirror = !mirror }
                if (p.qaItems > 0) Btn("Q&A") { showQA = !showQA }
                Btn("Stop", primary = true) { finish(false) }
            }
            // timeline
            Row(Modifier.fillMaxWidth().height(6.dp)) {
                main.paras.forEachIndexed { k, q ->
                    val f = when { k < mi -> 1f; k > mi -> 0f; else -> ((main.s - q.start) / q.plan).toFloat().coerceIn(0f, 1f) }
                    Box(Modifier.weight(q.plan.toFloat().coerceAtLeast(0.1f)).fillMaxSize().padding(end = 1.dp).background(Trail.copy(alpha = 0.4f))) {
                        Box(Modifier.fillMaxSize().graphicsLayer { scaleX = if (q.skipped) 0f else f; transformOrigin = androidx.compose.ui.graphics.TransformOrigin(0f, 0f) }
                            .background(if (q.actual > q.plan * 1.15 + 0.5) WarnC else Code))
                    }
                }
            }
        }
    }
}

@Composable
fun ReportScreen(m: AppModel, id: String, r: RunResult) {
    val run = r.run
    fun d(k: String) = (run[k] as? JsonPrimitive)?.content?.toDoubleOrNull() ?: 0.0
    val actual = d("actual"); val goal = if (d("target") > 0) d("target") else d("planned")
    val completed = (run["completed"] as? JsonPrimitive)?.content == "true"
    val diff = actual - goal
    Column(Modifier.fillMaxSize()) {
        TopBar(if (completed) "Run complete" else "Run stopped", onBack = { m.home() }) {
            Btn("Again", primary = true) { m.replace(Screen.Prompter(id)) }
        }
        Column(Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(14.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Text(fmt(actual), color = if (diff > 2) WarnC else Code, fontSize = 40.sp, fontFamily = Mono)
            Text(when {
                !completed -> "Stopped before the end. Planned ${fmt(d("planned"))}."
                abs(diff) <= 2 -> "On time against ${fmt(goal)}."
                diff > 0 -> "${fmt(diff)} over ${fmt(goal)}."
                else -> "${fmt(-diff)} under ${fmt(goal)}."
            }, color = ReadC)
            val paras = (run["paras"] as? JsonArray) ?: JsonArray(emptyList())
            paras.forEachIndexed { i, e ->
                val o = e as JsonObject
                val pl = (o["planned"] as JsonPrimitive).content.toDouble(); val ac = (o["actual"] as JsonPrimitive).content.toDouble()
                val skipped = o["skipped"] != null; val nr = o["notReached"] != null
                val late = !skipped && !nr && ac - pl > pl * 0.15 + 0.5
                Row(horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                    Text("${i + 1}", color = Ghost, fontFamily = Mono)
                    Text(r.texts.getOrNull(i) ?: "", color = ReadC, maxLines = 1, modifier = Modifier.weight(1f), fontSize = 13.sp)
                    Text(fmt(pl), color = Ghost, fontFamily = Mono)
                    Text(if (skipped) "skipped" else if (nr) "—" else fmt(ac), color = if (late) WarnC else ReadC, fontFamily = Mono)
                }
            }
            (run["answers"] as? JsonArray)?.let { a ->
                Text("Answers", color = Code, modifier = Modifier.padding(top = 8.dp))
                a.forEach { e -> val o = e as JsonObject
                    Hint("${(o["title"] as JsonPrimitive).content} · ${fmt((o["actual"] as JsonPrimitive).content.toDouble())} of ${fmt((o["planned"] as JsonPrimitive).content.toDouble())}")
                }
            }
            Text("Earlier runs", color = Code, modifier = Modifier.padding(top = 8.dp))
            m.repo.runs(id).drop(1).take(7).forEach { o ->
                val at = (o["at"] as? JsonPrimitive)?.content ?: ""
                val a = (o["actual"] as? JsonPrimitive)?.content?.toDoubleOrNull() ?: 0.0
                val dev = if ((o["device"] as? JsonPrimitive)?.content == "phone") "phone" else "PC"
                Hint("${at.replace('T', ' ').removeSuffix("Z")} — ${fmt(a)} · $dev")
            }
        }
    }
}
