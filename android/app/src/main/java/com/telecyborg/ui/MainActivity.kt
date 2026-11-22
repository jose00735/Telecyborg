package com.telecyborg.ui

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.BackHandler
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawingPadding
import androidx.compose.material3.Text
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.delay

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        val m = model(application)
        setContent {
            TcTheme {
                BackHandler(enabled = m.stack.size > 1) { m.back() }
                Box(Modifier.fillMaxSize().background(Void)) {
                    val inset = if (m.screen is Screen.Prompter) Modifier else Modifier.safeDrawingPadding()
                    Box(inset.fillMaxSize()) {
                        when (val s = m.screen) {
                            Screen.Library -> LibraryScreen(m)
                            is Screen.Editor -> EditorScreen(m, s.id)
                            is Screen.Prompter -> PrompterScreen(m, s.id)
                            is Screen.Report -> ReportScreen(m, s.id, s.run)
                            Screen.Packs -> PacksScreen(m)
                            is Screen.PackEdit -> PackScreen(m, s.id)
                            Screen.Sync -> SyncScreen(m)
                            Screen.Settings -> SettingsScreen(m)
                        }
                    }
                    val t = m.toast
                    if (t != null) {
                        LaunchedEffect(t) { delay(2800); if (m.toast == t) m.toast = null }
                        Text(t, color = Code, modifier = Modifier.align(Alignment.BottomCenter).safeDrawingPadding().padding(16.dp)
                            .border(1.dp, Code).background(PanelC).padding(12.dp))
                    }
                }
            }
        }
    }

    override fun onResume() {
        super.onResume()
        model(application).syncNow(quiet = true)
    }

    companion object {
        private var instance: AppModel? = null
        fun model(app: android.app.Application) = instance ?: AppModel(app).also { instance = it }
    }
}
