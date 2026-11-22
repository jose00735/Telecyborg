package com.telecyborg.ui

import android.app.Application
import app.cash.paparazzi.DeviceConfig
import app.cash.paparazzi.Paparazzi
import com.android.resources.ScreenOrientation
import org.junit.Rule
import org.junit.Test

/** Renders the main screens on the JVM (no emulator); images land in app/src/test/snapshots. */
class ScreensTest {
    @get:Rule
    val paparazzi = Paparazzi(deviceConfig = DeviceConfig.PIXEL_6.copy(orientation = ScreenOrientation.PORTRAIT), theme = "android:Theme.Material.NoActionBar")

    private fun model(): AppModel {
        val dir = java.nio.file.Files.createTempDirectory("tc").toFile()
        val app = object : Application() {
            override fun getFilesDir() = dir
        }
        val m = AppModel(app)
        m.repo.saveScript(null, "Terrific — 90 second pitch", java.io.File("../golden/pitch.md").readText(), 150, 90.0)
        m.repo.saveScript(null, "Sample — how TeleCyborg works", SAMPLE, 140, 0.0)
        m.refresh()
        return m
    }

    private val registry = object : androidx.activity.result.ActivityResultRegistryOwner {
        override val activityResultRegistry = object : androidx.activity.result.ActivityResultRegistry() {
            override fun <I, O> onLaunch(requestCode: Int, contract: androidx.activity.result.contract.ActivityResultContract<I, O>, input: I, options: androidx.core.app.ActivityOptionsCompat?) = Unit
        }
    }

    @Test fun library() {
        val m = model()
        paparazzi.snapshot {
            androidx.compose.runtime.CompositionLocalProvider(androidx.activity.compose.LocalActivityResultRegistryOwner provides registry) { TcTheme { LibraryScreen(m) } }
        }
    }
    @Test fun editor() { val m = model(); paparazzi.snapshot { TcTheme { EditorScreen(m, m.scripts.first { it.title.startsWith("Terrific") }.id) } } }
    @Test fun prompter() { val m = model(); paparazzi.snapshot { TcTheme { PrompterScreen(m, m.scripts.first { it.title.startsWith("Terrific") }.id) } } }
    @Test fun pack() {
        val m = model()
        val p = m.repo.savePack(com.telecyborg.core.Pack(title = "Terrific interview", topics = listOf("Netskope results", "Fortinet support"), facts = listOf("Moved an account from below 50% to 70% utilisation")))
        paparazzi.snapshot { TcTheme { PackScreen(m, p.id) } }
    }
    @Test fun settings() { val m = model(); paparazzi.snapshot { TcTheme { SettingsScreen(m) } } }
}
