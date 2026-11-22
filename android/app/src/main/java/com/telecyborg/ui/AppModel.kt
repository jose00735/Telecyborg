package com.telecyborg.ui

import android.app.Application
import android.os.Build
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import com.telecyborg.data.Repo
import com.telecyborg.data.Summary
import com.telecyborg.data.SyncedSettings
import com.telecyborg.sync.HubClient
import com.telecyborg.sync.PcLink
import com.telecyborg.sync.SyncEngine
import com.telecyborg.sync.SyncState
import com.telecyborg.sync.SyncStore
import com.telecyborg.sync.asRemote
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import java.io.File
import java.security.SecureRandom
import java.util.UUID

sealed class Screen {
    data object Library : Screen()
    data class Editor(val id: String?) : Screen()
    data class Prompter(val id: String) : Screen()
    data class Report(val id: String, val run: RunResult) : Screen()
    data object Packs : Screen()
    data class PackEdit(val id: String?) : Screen()
    data object Sync : Screen()
    data object Settings : Screen()
}

/** App state: storage, navigation, sync. One instance for the process. */
class AppModel(app: Application) {
    val repo = Repo(File(app.filesDir, "data"))
    private val syncStore = SyncStore(File(app.filesDir, "data/sync/state.json"))
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Main)

    val stack = mutableStateListOf<Screen>(Screen.Library)
    val screen get() = stack.last()
    var scripts by mutableStateOf<List<Summary>>(emptyList()); private set
    var settings by mutableStateOf(repo.settings()); private set
    var sync by mutableStateOf(syncStore.load()); private set
    var syncing by mutableStateOf(false); private set
    var syncMsg by mutableStateOf(""); private set
    var toast by mutableStateOf<String?>(null)

    val phoneId: String = File(app.filesDir, "data/sync/phone-id").let { f ->
        if (f.exists()) f.readText() else UUID.randomUUID().toString().also { f.parentFile?.mkdirs(); f.writeText(it) }
    }

    init { refresh() }

    fun refresh() { scripts = repo.list() }

    fun go(s: Screen) { stack.add(s) }
    fun back(): Boolean {
        if (stack.size <= 1) return false
        stack.removeAt(stack.lastIndex)
        if (screen == Screen.Library) refresh()
        return true
    }
    fun replace(s: Screen) { stack[stack.lastIndex] = s }
    fun home() { while (stack.size > 1) stack.removeAt(stack.lastIndex); refresh() }

    fun saveSettings(s: SyncedSettings) { settings = s; repo.saveSettings(s); changed() }

    // ---------- sync ----------
    private var pending: Job? = null

    /** Something changed locally: sync a few seconds later when paired. */
    fun changed() {
        refresh()
        if (sync.pc == null) return
        pending?.cancel()
        pending = scope.launch { delay(3000); syncNow(quiet = true) }
    }

    fun syncNow(quiet: Boolean = false) {
        val pc = sync.pc ?: return
        if (syncing) return
        syncing = true
        scope.launch {
            val msg = withContext(Dispatchers.IO) {
                runCatching {
                    val client = HubClient(pc.host, pc.port, pc.fp, pc.phoneId, pc.token)
                    val base = HashMap(sync.base)
                    val report = SyncEngine(repo, client.asRemote(), base).run()
                    val st = sync.copy(base = base, lastSync = System.currentTimeMillis(), lastResult = report.toString())
                    syncStore.save(st)
                    st to report.toString()
                }
            }
            syncing = false
            msg.onSuccess { (st, text) ->
                sync = st; syncMsg = text
                settings = repo.settings(); refresh()
                if (!quiet || text != "Up to date") toast = "Sync: $text"
            }.onFailure { e ->
                syncMsg = friendly(e)
                if (!quiet) toast = syncMsg
            }
        }
    }

    private fun friendly(e: Throwable): String = when {
        e.message?.contains("certificate changed") == true -> "The PC's identity changed. Pair again if you reinstalled TeleCyborg on the PC."
        e is java.net.ConnectException || e is java.net.SocketTimeoutException || e is java.net.NoRouteToHostException ->
            "PC not reachable. Same Wi-Fi? TeleCyborg open on the PC with Phone sync on?"
        e is com.telecyborg.sync.HubException && e.code == 401 -> "This PC forgot the phone. Pair again."
        else -> e.message ?: e.toString()
    }

    // ---------- pairing ----------
    data class PairProgress(val stage: String, val code: String = "", val pcName: String = "", val error: String = "")
    var pair by mutableStateOf<PairProgress?>(null)
    private var pairClient: HubClient? = null
    private var pairStart: HubClient.PairStart? = null
    private var pairTarget: Pair<String, Int>? = null

    fun startPairing(host: String, port: Int) {
        pair = PairProgress("connecting")
        pairTarget = host to port
        scope.launch {
            val r = withContext(Dispatchers.IO) {
                runCatching {
                    val c = HubClient(host, port, null)
                    val name = "${Build.MANUFACTURER.replaceFirstChar { it.uppercase() }} ${Build.MODEL}"
                    val start = c.pairBegin(phoneId, name)
                    val code = c.pairNonce(start, ByteArray(32).also { SecureRandom().nextBytes(it) })
                    pairClient = c; pairStart = start
                    PairProgress("compare", code, start.pcName)
                }
            }
            pair = r.getOrElse { PairProgress("error", error = it.message ?: it.toString()) }
        }
    }

    fun answerPairing(ok: Boolean) {
        val c = pairClient ?: return
        val start = pairStart ?: return
        val (host, port) = pairTarget ?: return
        val cur = pair ?: return
        pair = cur.copy(stage = if (ok) "waiting-pc" else "cancelled")
        scope.launch {
            val r = withContext(Dispatchers.IO) { runCatching { c.pairConfirm(start, ok) } }
            val token = r.getOrNull()
            if (ok && token != null) {
                val link = PcLink(host, port, start.pcName, c.seenFp, phoneId, token)
                sync = SyncState(pc = link)
                syncStore.save(sync)
                pair = PairProgress("done", pcName = start.pcName)
                syncNow()
            } else {
                pair = PairProgress("error", error = r.exceptionOrNull()?.message ?: if (ok) "The PC said no." else "Cancelled.")
            }
        }
    }

    fun forgetPc() {
        sync = SyncState()
        syncStore.save(sync)
        syncMsg = ""
    }
}
