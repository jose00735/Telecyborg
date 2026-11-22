package com.telecyborg.ui

import android.net.Uri
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Slider
import androidx.compose.material3.SliderDefaults
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.telecyborg.core.Script

@Composable
fun LibraryScreen(m: AppModel) {
    val ctx = LocalContext.current
    val importer = rememberLauncherForActivityResult(ActivityResultContracts.OpenDocument()) { uri: Uri? ->
        if (uri != null) runCatching {
            val text = ctx.contentResolver.openInputStream(uri)!!.use { it.readBytes().decodeToString() }
            val name = uri.lastPathSegment?.substringAfterLast('/') ?: "script.md"
            val id = m.repo.importScript(name, text)
            m.changed(); m.go(Screen.Editor(id))
        }.onFailure { m.toast = "Could not import: ${it.message}" }
    }
    Column(Modifier.fillMaxSize()) {
        TopBar("TeleCyborg") {
            Btn("+ New", primary = true) { m.go(Screen.Editor(null)) }
        }
        Row(Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()).padding(horizontal = 12.dp), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            Btn("Content packs") { m.go(Screen.Packs) }
            Btn("Import .md") { importer.launch(arrayOf("text/*", "application/octet-stream")) }
            Btn(if (m.sync.pc != null) "Sync ⟳" else "Pair with PC") { if (m.sync.pc != null) m.syncNow() else m.go(Screen.Sync) }
            Btn("Settings") { m.go(Screen.Settings) }
        }
        val pc = m.sync.pc
        Column(Modifier.padding(horizontal = 14.dp, vertical = 6.dp)) { Hint(
            when {
                m.syncing -> "Syncing with ${pc?.name}…"
                pc == null -> "Works on its own. Pair with your PC to sync scripts, packs and settings."
                else -> "Hub: ${pc.name} · " + (if (m.sync.lastSync > 0) "last sync ${ago(m.sync.lastSync)}" else "not synced yet") + (if (m.syncMsg.isNotEmpty()) " · ${m.syncMsg}" else "")
            }, color = if (m.syncMsg.contains("not reachable") || m.syncMsg.contains("Pair again")) WarnC else Ghost,
        ) }
        if (m.scripts.isEmpty()) {
            Column(Modifier.padding(16.dp).border(1.dp, Trail).padding(16.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
                Text("No scripts yet.", color = Code, fontSize = 18.sp)
                Hint("Write a pitch, import a Markdown file, build one with Claude from a content pack, or pair with your PC to bring your scripts here.")
                Btn("Load the sample") {
                    val id = m.repo.saveScript(null, "Sample — how TeleCyborg works", SAMPLE, 140, 0.0)
                    m.changed(); m.go(Screen.Prompter(id))
                }
            }
        }
        LazyColumn(Modifier.fillMaxSize().padding(horizontal = 12.dp)) {
            items(m.scripts, key = { it.id }) { s ->
                Row(Modifier.fillMaxWidth().clickable { m.go(Screen.Prompter(s.id)) }.padding(vertical = 10.dp), verticalAlignment = Alignment.CenterVertically) {
                    Text(">", color = Code, modifier = Modifier.width(18.dp))
                    Column(Modifier.weight(1f)) {
                        Text(s.title, color = ReadC, fontSize = 16.sp, maxLines = 1, overflow = TextOverflow.Ellipsis)
                        Hint("${s.words} words · ${s.wpm} wpm" + (if (s.qa > 0) " · ${s.qa} Q&A" else "") + (if (s.target > 0) " · target ${fmt(s.target)}" else "") + (if (s.runs > 0) " · ${s.runs} runs" else "") + (if (s.pack.isNotEmpty()) " · pack" else ""))
                    }
                    Text(fmt(s.duration), color = Code, fontSize = 18.sp)
                    Btn("Edit", modifier = Modifier.padding(start = 8.dp)) { m.go(Screen.Editor(s.id)) }
                }
            }
        }
    }
}

fun ago(ms: Long): String {
    val d = (System.currentTimeMillis() - ms) / 1000
    return when {
        d < 60 -> "just now"
        d < 3600 -> "${d / 60} min ago"
        d < 86400 -> "${d / 3600} h ago"
        else -> "${d / 86400} d ago"
    }
}

@Composable
fun EditorScreen(m: AppModel, id: String?) {
    val doc = remember(id) { id?.let { m.repo.get(it) } }
    var title by remember(id) { mutableStateOf(doc?.title ?: "") }
    var body by remember(id) { mutableStateOf(doc?.body ?: "") }
    var wpm by remember(id) { mutableStateOf((doc?.wpm ?: m.settings.wpm).toFloat()) }
    var target by remember(id) { mutableStateOf(doc?.target?.takeIf { it > 0 }?.let(::fmt) ?: "") }
    var savedId by remember(id) { mutableStateOf(id) }
    var msg by remember(id) { mutableStateOf("") }
    var confirmDelete by remember(id) { mutableStateOf(false) }
    val parsed = remember(body) { Script.parse(body) }

    fun save(): String? {
        if (body.isBlank()) { m.toast = "Write or paste some text first"; return null }
        val t = Script.parseDuration(target)
        val sid = m.repo.saveScript(savedId, title, body, wpm.toInt(), t)
        savedId = sid
        title = m.repo.get(sid)?.title ?: title
        m.changed(); m.toast = "Saved"
        return sid
    }

    Column(Modifier.fillMaxSize()) {
        TopBar(if (savedId == null) "New script" else "Edit", onBack = { if (body.isNotBlank()) save(); m.back() }) {
            Btn("Save") { save() }
            Btn("Rehearse", primary = true) { save()?.let { m.replace(Screen.Prompter(it)) } }
        }
        Column(Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(12.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
            Field("Title", title) { title = it }
            Field("Script — # heading, blank line between paragraphs, [pause 2], **stress**, > cue", body, Modifier.fillMaxWidth().heightIn(min = 260.dp), lines = 12) { body = it }
            Box1("Pace") {
                Text("${wpm.toInt()} wpm", color = Code)
                Slider(wpm, { wpm = it }, valueRange = 60f..300f, colors = SliderDefaults.colors(thumbColor = Code, activeTrackColor = Code, inactiveTrackColor = Trail))
                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    Field("Target m:ss", target, Modifier.width(140.dp)) { target = it }
                    Btn("Fit to time") {
                        val t = Script.parseDuration(target)
                        if (t <= 0 || parsed.words == 0) msg = "Type a target like 1:30 and some text first."
                        else { val raw = parsed.words * 60.0 / (t - parsed.pauses); wpm = Script.clampWpm(raw.toInt()).toFloat()
                            msg = if (raw > 300 || raw <= 0) "Needs more than 300 wpm: cut text or allow more time." else "Set to ${wpm.toInt()} wpm to finish in ${fmt(t)}." }
                    }
                }
                if (msg.isNotEmpty()) Hint(msg)
            }
            Box1("Stats") {
                Hint("${parsed.words} words · ${parsed.paragraphs} paragraphs · ${parsed.sections - parsed.qaItems} sections · ${parsed.qaItems} Q&A answers · pauses ${parsed.pauses} s")
                if (parsed.checks > 0) Hint("${parsed.checks} [check] mark(s): facts to verify before you present.", WarnC)
                Text("Estimated ${fmt(parsed.words * 60.0 / wpm + parsed.pauses)}", color = Code, fontSize = 22.sp, fontWeight = FontWeight.Bold)
            }
            if (parsed.outline.isNotEmpty()) Box1("Sections & cue keywords") {
                parsed.outline.forEach { o -> Hint((if (o.qa) "Q&A · " else "") + o.title + if (o.keywords.isNotEmpty()) " — " + o.keywords.joinToString(", ") else "", if (o.qa) WarnC else Ghost) }
            }
            if (savedId != null) {
                Btn(if (confirmDelete) "Tap again to delete" else "Delete script") {
                    if (!confirmDelete) confirmDelete = true else { m.repo.deleteScript(savedId!!); m.changed(); m.home() }
                }
            }
        }
    }
}

const val SAMPLE = """# Opening
This is TeleCyborg on your phone. Read along the bright line. The text moves at the pace you set, and every paragraph has a time budget.

[pause 1.5]

# Controls
Tap the text to pause and play. Swipe up or down to change paragraph. The buttons change the pace and the text size.
> breathe

# Pace coaching
If you fall behind the target time, an amber bar tells you by how much and the pace you need. Tap it to apply.

# Sync
Pair with TeleCyborg on your PC to keep scripts, content packs and settings in step. Your phone keeps working without the PC.

=== QA ===
# Q: Can I answer a question and come back?
keywords: question, answer
Yes. Open Q&A, pick the answer, and tap back to return to where you were."""
