package com.telecyborg.ui

import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.content.Intent
import android.net.nsd.NsdManager
import android.net.nsd.NsdServiceInfo
import android.net.wifi.WifiManager
import android.os.Build
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Slider
import androidx.compose.material3.SliderDefaults
import androidx.compose.material3.Switch
import androidx.compose.material3.SwitchDefaults
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.telecyborg.core.Pack
import com.telecyborg.core.Packs
import com.telecyborg.core.Preview
import java.util.concurrent.CountDownLatch
import java.util.concurrent.Executors
import java.util.concurrent.TimeUnit

// ---------------- content packs ----------------

@Composable
fun PacksScreen(m: AppModel) {
    val packs = remember(m.scripts) { m.repo.listPacks() }
    Column(Modifier.fillMaxSize()) {
        TopBar("Content packs", onBack = { m.back() }) { Btn("+ New", primary = true) { m.go(Screen.PackEdit(null)) } }
        Column(Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(12.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
            Hint("Describe the occasion, send the brief to Claude, paste Claude's answer back: you get a timed pitch plus likely questions with answers.")
            if (packs.isEmpty()) Hint("No packs yet.", ReadC)
            packs.forEach { p ->
                Row(Modifier.fillMaxWidth().clickable { m.go(Screen.PackEdit(p.id)) }.padding(vertical = 8.dp), verticalAlignment = Alignment.CenterVertically) {
                    Column(Modifier.weight(1f)) {
                        Text(p.title, color = ReadC, fontSize = 16.sp)
                        Hint("${p.occasion} · ${p.target} · ${p.topics.size} topics · ${p.qaCount} questions · " + if (p.scriptId.isNotEmpty()) "script ready" else "waiting for Claude's answer")
                    }
                    if (p.scriptId.isNotEmpty()) Btn("Rehearse", primary = true) { m.go(Screen.Prompter(p.scriptId)) }
                }
            }
        }
    }
}

@Composable
fun PackScreen(m: AppModel, id: String?) {
    val ctx = LocalContext.current
    val doc = remember(id) { id?.let { m.repo.pack(it) } }
    var p by remember(id) { mutableStateOf(doc?.pack ?: Pack()) }
    var topics by remember(id) { mutableStateOf(p.topics.joinToString("\n")) }
    var facts by remember(id) { mutableStateOf(p.facts.joinToString("\n")) }
    var avoid by remember(id) { mutableStateOf(p.avoid.joinToString("\n")) }
    var brief by remember(id) { mutableStateOf(if (doc != null) Packs.brief(doc.pack) else "") }
    var answer by remember(id) { mutableStateOf("") }
    var preview by remember(id) { mutableStateOf<Preview?>(null) }
    var confirmDelete by remember(id) { mutableStateOf(false) }

    fun current() = p.copy(topics = topics.lines(), facts = facts.lines(), avoid = avoid.lines())
    fun save(): Pack? {
        if (p.title.isBlank()) { m.toast = "Give the pack a title"; return null }
        val saved = m.repo.savePack(current())
        p = saved; brief = Packs.brief(saved); m.changed()
        return saved
    }

    Column(Modifier.fillMaxSize()) {
        TopBar(if (id == null) "New pack" else "Pack", onBack = { if (p.title.isNotBlank()) save(); m.back() }) { Btn("Save") { if (save() != null) m.toast = "Saved" } }
        Column(Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(12.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
            Field("Title", p.title) { p = p.copy(title = it) }
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                listOf("interview" to "Interview", "customer meeting" to "Meeting", "demo" to "Demo").forEach { (v, l) ->
                    Btn(if (p.occasion == v) "● $l" else l) { p = p.copy(occasion = v) }
                }
            }
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                listOf("en" to "EN", "es" to "ES", "pt" to "PT").forEach { (v, l) -> Btn(if (p.lang == v) "● $l" else l) { p = p.copy(lang = v) } }
            }
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Field("Length m:ss", p.target, Modifier.weight(1f)) { p = p.copy(target = it) }
                Field("wpm", p.wpm.toString(), Modifier.weight(1f), number = true) { p = p.copy(wpm = it.toIntOrNull() ?: 0) }
                Field("Questions", p.qaCount.toString(), Modifier.weight(1f), number = true) { p = p.copy(qaCount = it.toIntOrNull() ?: 0) }
            }
            Field("Audience", p.audience) { p = p.copy(audience = it) }
            Field("Goal", p.goal) { p = p.copy(goal = it) }
            Field("Topics, one per line", topics, lines = 3) { topics = it }
            Field("Facts to mention, one per line", facts, lines = 3) { facts = it }
            Field("Avoid, one per line", avoid, lines = 2) { avoid = it }
            Field("Sources: links or pasted text", p.sources, lines = 4) { p = p.copy(sources = it) }

            Box1("1 · Brief for Claude") {
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    Btn("Save & write brief", primary = true) { save() }
                    Btn("Copy", enabled = brief.isNotEmpty()) {
                        (ctx.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager).setPrimaryClip(ClipData.newPlainText("TeleCyborg brief", brief))
                        m.toast = "Brief copied"
                    }
                    Btn("Share to Claude", enabled = brief.isNotEmpty()) {
                        ctx.startActivity(Intent.createChooser(Intent(Intent.ACTION_SEND).setType("text/plain").putExtra(Intent.EXTRA_TEXT, brief), "Send the brief"))
                    }
                }
                if (brief.isNotEmpty()) Text(brief.take(600) + if (brief.length > 600) "…" else "", color = Ghost, fontSize = 11.sp, fontFamily = Mono)
            }
            Box1("2 · Paste Claude's answer") {
                Field("Claude's whole reply", answer, Modifier.fillMaxWidth().heightIn(min = 140.dp), lines = 6) { answer = it; preview = null }
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    Btn("Check") { val s = save() ?: return@Btn; preview = Packs.validate(answer, s) }
                    Btn(if (p.scriptId.isNotEmpty()) "Update script" else "Create script", primary = true, enabled = preview?.ok == true) {
                        val s = save() ?: return@Btn
                        val (sid, pv) = m.repo.importAnswer(s.id, answer)
                        preview = pv
                        if (sid != null) { p = m.repo.pack(s.id)?.pack ?: p; m.changed(); m.toast = "Script ready"; m.go(Screen.Editor(sid)) }
                    }
                }
                preview?.let { pv ->
                    Hint("${pv.sections} sections · ${pv.qa} questions · ${pv.words} words · ${fmt(pv.duration)} of ${fmt(pv.target)} · ${pv.checks} [check]", ReadC)
                    pv.errors.forEach { Hint("✕ $it", Miss) }
                    pv.warnings.forEach { Hint("! $it", WarnC) }
                }
            }
            if (p.scriptId.isNotEmpty()) Btn("Rehearse the script", primary = true) { m.go(Screen.Prompter(p.scriptId)) }
            if (id != null) Btn(if (confirmDelete) "Tap again to delete the pack" else "Delete pack") {
                if (!confirmDelete) confirmDelete = true else { m.repo.deletePack(id); m.changed(); m.back() }
            }
        }
    }
}

// ---------------- sync with the PC ----------------

data class FoundPc(val name: String, val host: String, val port: Int, val fp: String)

/** Finds TeleCyborg PCs on the LAN over mDNS (_telecyborg._tcp), like Macawlink's discovery. */
class PcFinder(context: Context) {
    private val nsd = context.getSystemService(NsdManager::class.java)
    private val wifi = context.applicationContext.getSystemService(WifiManager::class.java)
    private var lock: WifiManager.MulticastLock? = null
    private var listener: NsdManager.DiscoveryListener? = null
    private val resolver = Executors.newSingleThreadExecutor()
    val found = mutableStateListOf<FoundPc>()
    private val main = android.os.Handler(android.os.Looper.getMainLooper())

    fun start() {
        if (listener != null) return
        lock = wifi.createMulticastLock("telecyborg-mdns").apply { setReferenceCounted(false); acquire() }
        val l = object : NsdManager.DiscoveryListener {
            override fun onDiscoveryStarted(t: String) = Unit
            override fun onDiscoveryStopped(t: String) = Unit
            override fun onStartDiscoveryFailed(t: String, e: Int) = Unit
            override fun onStopDiscoveryFailed(t: String, e: Int) = Unit
            override fun onServiceFound(info: NsdServiceInfo) { resolver.execute { resolve(info) } }
            override fun onServiceLost(info: NsdServiceInfo) { main.post { found.removeAll { it.name == info.serviceName.removePrefix("TeleCyborg on ") } } }
        }
        listener = l
        runCatching { nsd.discoverServices("_telecyborg._tcp", NsdManager.PROTOCOL_DNS_SD, l) }
    }

    @Suppress("DEPRECATION")
    private fun resolve(info: NsdServiceInfo) {
        val done = CountDownLatch(1)
        nsd.resolveService(info, object : NsdManager.ResolveListener {
            override fun onResolveFailed(si: NsdServiceInfo, e: Int) { done.countDown() }
            override fun onServiceResolved(si: NsdServiceInfo) {
                val host = if (Build.VERSION.SDK_INT >= 34) si.hostAddresses.firstOrNull()?.hostAddress else si.host?.hostAddress
                val name = si.attributes["name"]?.decodeToString() ?: si.serviceName.removePrefix("TeleCyborg on ")
                val fp = si.attributes["fp"]?.decodeToString() ?: ""
                if (host != null) main.post { found.removeAll { it.name == name }; found.add(FoundPc(name, host, si.port, fp)) }
                done.countDown()
            }
        })
        done.await(5, TimeUnit.SECONDS)
    }

    fun stop() {
        listener?.let { runCatching { nsd.stopServiceDiscovery(it) } }
        listener = null
        lock?.let { if (it.isHeld) it.release() }
    }
}

@Composable
fun SyncScreen(m: AppModel) {
    val ctx = LocalContext.current
    val finder = remember { PcFinder(ctx) }
    DisposableEffect(Unit) { finder.start(); onDispose { finder.stop() } }
    var manual by remember { mutableStateOf("") }
    Column(Modifier.fillMaxSize()) {
        TopBar("Sync with PC", onBack = { m.pair = null; m.back() })
        Column(Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(12.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
            Hint("Your PC is the hub. This phone works on its own and syncs scripts, content packs, settings and rehearsal history with it on the same Wi-Fi.")
            val pc = m.sync.pc
            if (pc != null) Box1("Paired with ${pc.name}") {
                Hint("${pc.host}:${pc.port} · identity ${pc.fp.take(8)}" + if (m.sync.lastSync > 0) " · last sync ${ago(m.sync.lastSync)}: ${m.sync.lastResult}" else "")
                if (m.syncMsg.isNotEmpty()) Hint(m.syncMsg, if (m.syncMsg.contains("reachable") || m.syncMsg.contains("Pair")) WarnC else Ghost)
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    Btn(if (m.syncing) "Syncing…" else "Sync now", primary = true, enabled = !m.syncing) { m.syncNow() }
                    Btn("Forget PC") { m.forgetPc() }
                }
            }
            val pr = m.pair
            when (pr?.stage) {
                "connecting" -> Box1("Pairing") { Hint("Contacting the PC…", ReadC) }
                "compare" -> Box1("Does ${pr.pcName} show this code?") {
                    Text(pr.code.chunked(3).joinToString(" "), color = Code, fontSize = 44.sp, fontFamily = Mono)
                    Hint("Accept on both screens only if the codes match.")
                    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        Btn("No") { m.answerPairing(false) }
                        Btn("Yes, it matches", primary = true) { m.answerPairing(true) }
                    }
                }
                "waiting-pc" -> Box1("Pairing") { Hint("Now confirm on the PC…", ReadC) }
                "done" -> Box1("Paired") { Hint("Paired with ${pr.pcName}. Syncing now.", Code) }
                "error", "cancelled" -> Box1("Pairing stopped") { Hint(pr.error, WarnC) }
                else -> {}
            }
            Box1(if (pc == null) "Pair with your PC" else "Pair with another PC") {
                Hint("On the PC: TeleCyborg → Phone sync → Pair a phone. Then choose it here.")
                if (finder.found.isEmpty()) Hint("Looking for PCs on this Wi-Fi…")
                finder.found.forEach { f ->
                    Row(Modifier.fillMaxWidth().border(1.dp, Trail).clickable { m.startPairing(f.host, f.port) }.padding(10.dp), verticalAlignment = Alignment.CenterVertically) {
                        Column(Modifier.weight(1f)) { Text(f.name, color = ReadC); Hint("${f.host}:${f.port} · ${f.fp}") }
                        Text("Pair ›", color = Code)
                    }
                }
                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    Field("Or type the address (192.168.1.20:47811)", manual, Modifier.weight(1f)) { manual = it.trim() }
                    Btn("Pair") {
                        val host = manual.substringBeforeLast(':').ifEmpty { manual }
                        val port = manual.substringAfterLast(':', "47811").toIntOrNull() ?: 47811
                        if (host.isNotEmpty()) m.startPairing(host, port)
                    }
                }
            }
        }
    }
}

// ---------------- settings ----------------

@Composable
fun SettingsScreen(m: AppModel) {
    var s by remember { mutableStateOf(m.settings) }
    fun set(n: com.telecyborg.data.SyncedSettings) { s = n; m.saveSettings(n) }
    val colors = SliderDefaults.colors(thumbColor = Code, activeTrackColor = Code, inactiveTrackColor = Trail)
    Column(Modifier.fillMaxSize()) {
        TopBar("Settings", onBack = { m.back() })
        Column(Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(12.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
            Hint("These follow you between the phone and the PC.")
            Box1("Default pace: ${s.wpm} wpm") { Slider(s.wpm.toFloat(), { set(s.copy(wpm = it.toInt())) }, valueRange = 60f..300f, colors = colors) }
            Box1("Text size: ${s.fontSize}") { Slider(s.fontSize.toFloat(), { set(s.copy(fontSize = it.toInt())) }, valueRange = 24f..110f, colors = colors) }
            Box1("Focus line: ${(s.focusLine * 100).toInt()}% from the top") { Slider(s.focusLine.toFloat(), { set(s.copy(focusLine = (it * 100).toInt() / 100.0)) }, valueRange = 0.2f..0.6f, colors = colors) }
            Box1("Countdown: ${s.countdown} s") { Slider(s.countdown.toFloat(), { set(s.copy(countdown = it.toInt())) }, valueRange = 0f..10f, steps = 9, colors = colors) }
            Box1("Answer length guard: ${s.answerMax} s") { Slider(s.answerMax.toFloat(), { set(s.copy(answerMax = it.toInt())) }, valueRange = 20f..180f, colors = colors) }
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text("Mirror text (beam-splitter glass)", color = ReadC, modifier = Modifier.weight(1f))
                Switch(s.mirror, { set(s.copy(mirror = it)) }, colors = SwitchDefaults.colors(checkedThumbColor = Void, checkedTrackColor = Code))
            }
            Hint("TeleCyborg for Android ${com.telecyborg.BuildConfigShim.VERSION} · listening and smart follow run on the PC.")
            Row(Modifier.width(1.dp)) {}
        }
    }
}
