package com.telecyborg.ui

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.RowScope
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.OutlinedTextFieldDefaults
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.darkColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp

// Same tokens as the PC (docs/requirements/05-ui-matrix-design.md).
val Void = Color(0xFF000500)
val PanelC = Color(0xFF010D03)
val Code = Color(0xFF00FF41)
val Trail = Color(0xFF0B6E2A)
val ReadC = Color(0xFFD8FFE0)
val Ghost = Color(0xFF5F8F69)
val WarnC = Color(0xFFFFB000)
val Miss = Color(0xFFFF4D4D)
val Mono = FontFamily.Monospace

@Composable
fun TcTheme(content: @Composable () -> Unit) {
    MaterialTheme(
        colorScheme = darkColorScheme(primary = Code, onPrimary = Void, secondary = WarnC, background = Void, surface = PanelC,
            onBackground = ReadC, onSurface = ReadC, outline = Trail),
        typography = MaterialTheme.typography.copy(
            bodyLarge = TextStyle(fontFamily = Mono, fontSize = 15.sp, color = ReadC),
            bodyMedium = TextStyle(fontFamily = Mono, fontSize = 14.sp, color = ReadC),
        ),
        content = content,
    )
}

@Composable
fun TopBar(title: String, onBack: (() -> Unit)? = null, actions: @Composable RowScope.() -> Unit = {}) {
    Row(Modifier.fillMaxWidth().padding(horizontal = 8.dp, vertical = 6.dp), verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(6.dp)) {
        if (onBack != null) TextButton(onClick = onBack) { Text("‹", color = Ghost, fontSize = 22.sp) }
        Text(title, color = Code, fontFamily = Mono, fontSize = 22.sp, fontWeight = FontWeight.Bold, modifier = Modifier.weight(1f), maxLines = 1)
        actions()
    }
}

@Composable
fun Btn(text: String, primary: Boolean = false, enabled: Boolean = true, modifier: Modifier = Modifier, onClick: () -> Unit) {
    OutlinedButton(onClick = onClick, enabled = enabled, modifier = modifier, shape = RoundedCornerShape(2.dp),
        border = BorderStroke(1.dp, if (primary) Code else Trail),
        colors = ButtonDefaults.outlinedButtonColors(containerColor = if (primary) Code else Color.Transparent, contentColor = if (primary) Void else ReadC)) {
        Text(text, fontFamily = Mono, fontWeight = if (primary) FontWeight.Bold else FontWeight.Normal, fontSize = 14.sp)
    }
}

@Composable
fun Field(label: String, value: String, modifier: Modifier = Modifier.fillMaxWidth(), lines: Int = 1, number: Boolean = false, onChange: (String) -> Unit) {
    OutlinedTextField(value = value, onValueChange = onChange, label = { Text(label, fontFamily = Mono, fontSize = 12.sp) }, modifier = modifier,
        singleLine = lines == 1, minLines = lines, textStyle = TextStyle(fontFamily = Mono, fontSize = 15.sp, color = ReadC),
        keyboardOptions = if (number) KeyboardOptions(keyboardType = KeyboardType.Number) else KeyboardOptions.Default,
        colors = OutlinedTextFieldDefaults.colors(focusedBorderColor = Code, unfocusedBorderColor = Trail, cursorColor = Code, focusedLabelColor = Code))
}

@Composable
fun Box1(title: String? = null, content: @Composable () -> Unit) {
    Column(Modifier.fillMaxWidth().border(1.dp, Trail).background(PanelC).padding(12.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
        if (title != null) Text(title, color = Code, fontFamily = Mono, fontSize = 15.sp)
        content()
    }
}

@Composable
fun Hint(text: String, color: Color = Ghost) = Text(text, color = color, fontFamily = Mono, fontSize = 12.sp)

fun fmt(sec: Double): String {
    if (!sec.isFinite()) return "--:--"
    val neg = sec < 0
    val s = kotlin.math.abs(Math.round(sec)).toInt()
    return (if (neg) "−" else "") + "${s / 60}:${(s % 60).toString().padStart(2, '0')}"
}
