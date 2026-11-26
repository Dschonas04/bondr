package io.github.dschonas04.bondr

import android.Manifest
import android.app.Activity
import android.content.pm.PackageManager
import android.net.VpnService
import android.os.Build
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.animation.animateContentSize
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.NetworkCell
import androidx.compose.material.icons.outlined.Wifi
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SegmentedButton
import androidx.compose.material3.SegmentedButtonDefaults
import androidx.compose.material3.SingleChoiceSegmentedButtonRow
import androidx.compose.material3.Slider
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableFloatStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.geometry.CornerRadius
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.core.content.ContextCompat
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import org.json.JSONObject

// ---------------------------------------------------------------------------------------
// Daten, wie der Kern sie meldet

data class Weg(
    val name: String, val art: String, val ip: String, val geraet: String,
    val gewicht: Int, val rolle: String, val benutzt: Boolean, val gestoert: Boolean,
    val fehler: String?, val offen: Int, val mbitRein: Double, val mbitRaus: Double,
)

data class Verbrauch(val monat: String, val monatBytes: Long, val gesamtBytes: Long, val seit: String)

data class Stand(
    val version: String, val strategie: String, val mbitRein: Double, val mbitRaus: Double,
    val wege: List<Weg>, val verbrauch: Map<String, Verbrauch>,
)

fun verbrauchLesen(o: JSONObject): Map<String, Verbrauch> = o.keys().asSequence().associateWith { k ->
    val v = o.getJSONObject(k)
    Verbrauch(v.optString("monat"), v.optLong("monat_rein") + v.optLong("monat_raus"),
        v.optLong("gesamt_rein") + v.optLong("gesamt_raus"), v.optString("seit"))
}

fun standLesen(o: JSONObject): Stand {
    val w = o.getJSONArray("wege")
    return Stand(
        o.optString("version"), o.optString("strategie"), o.optDouble("mbit_rein"), o.optDouble("mbit_raus"),
        (0 until w.length()).map { i ->
            val x = w.getJSONObject(i)
            Weg(x.optString("name"), x.optString("art"), x.optString("ip"), x.optString("beschreibung"),
                x.optInt("gewicht"), x.optString("rolle"), x.optBoolean("benutzt"), x.optBoolean("gestoert"),
                x.optString("fehler").ifEmpty { null }, x.optInt("offen"),
                x.optDouble("mbit_rein"), x.optDouble("mbit_raus"))
        },
        verbrauchLesen(o.optJSONObject("verbrauch") ?: JSONObject()),
    )
}

// ---------------------------------------------------------------------------------------
// Gestaltung

private val Akzent = Color(0xFF3B5BDB)
private val FarbeWlan = Color(0xFF1C7ED6)
private val FarbeMobil = Color(0xFF7048E8)
private val Gut = Color(0xFF2F9E44)
private val Warnung = Color(0xFFE8590C)
private val Schlecht = Color(0xFFE03131)

private val Hell = lightColorScheme(
    primary = Akzent, onPrimary = Color.White,
    background = Color(0xFFF6F7F9), surface = Color.White, surfaceVariant = Color(0xFFEEF0F4),
    onSurface = Color(0xFF15181D), onSurfaceVariant = Color(0xFF5F6670), outlineVariant = Color(0xFFE1E4EA),
)
private val Dunkel = darkColorScheme(
    primary = Color(0xFF7C94F4), onPrimary = Color(0xFF0E1013),
    background = Color(0xFF0E1013), surface = Color(0xFF171A1F), surfaceVariant = Color(0xFF22262D),
    onSurface = Color(0xFFE8EAED), onSurfaceVariant = Color(0xFF9AA1AB), outlineVariant = Color(0xFF2B3038),
)

fun farbe(art: String) = if (art == "WLAN") FarbeWlan else FarbeMobil
fun symbol(art: String): ImageVector = if (art == "WLAN") Icons.Outlined.Wifi else Icons.Outlined.NetworkCell

// ---------------------------------------------------------------------------------------

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        setContent {
            MaterialTheme(colorScheme = if (isSystemInDarkTheme()) Dunkel else Hell) { App() }
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun App() {
    val context = LocalContext.current
    val kern = remember { Kern(context) }
    val laeuft by BondrDienst.laeuft.collectAsState()
    val fehler by BondrDienst.fehler.collectAsState()
    var stand by remember { mutableStateOf<Stand?>(null) }
    var gespeichert by remember { mutableStateOf<Map<String, Verbrauch>>(emptyMap()) }
    var meldung by remember { mutableStateOf<String?>(null) }
    var zuruecksetzen by remember { mutableStateOf(false) }
    val scope = rememberCoroutineScope()

    val vpnErlauben = rememberLauncherForActivityResult(ActivityResultContracts.StartActivityForResult()) {
        if (it.resultCode == Activity.RESULT_OK) BondrDienst.starten(context)
    }
    val hinweise = rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) {}

    fun starten() {
        if (Build.VERSION.SDK_INT >= 33 && ContextCompat.checkSelfPermission(context,
                Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED) {
            hinweise.launch(Manifest.permission.POST_NOTIFICATIONS)
        }
        val i = VpnService.prepare(context)
        if (i != null) vpnErlauben.launch(i) else BondrDienst.starten(context)
    }

    fun senden(pfad: String, inhalt: JSONObject) = scope.launch {
        val r = withContext(Dispatchers.IO) { runCatching { kern.anfrage("POST", pfad, inhalt.toString()) } }
        meldung = r.exceptionOrNull()?.message
    }

    LaunchedEffect(laeuft) {
        while (true) {
            if (laeuft) {
                val r = withContext(Dispatchers.IO) { runCatching { standLesen(kern.stand()) } }
                r.onSuccess { stand = it; meldung = null }
            } else {
                stand = null
                gespeichert = withContext(Dispatchers.IO) { verbrauchLesen(kern.verbrauchGespeichert()) }
            }
            delay(1000)
        }
    }

    val verbrauch = stand?.verbrauch ?: gespeichert

    Scaffold(
        containerColor = MaterialTheme.colorScheme.background,
        topBar = {
            TopAppBar(
                title = {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Logo(Modifier.size(30.dp))
                        Spacer(Modifier.width(12.dp))
                        Text("Bondr", fontWeight = FontWeight.SemiBold)
                    }
                },
                colors = TopAppBarDefaults.topAppBarColors(containerColor = MaterialTheme.colorScheme.background),
            )
        },
    ) { innen ->
        LazyColumn(
            Modifier.fillMaxSize(),
            contentPadding = PaddingValues(start = 16.dp, end = 16.dp, top = innen.calculateTopPadding() + 4.dp,
                bottom = innen.calculateBottomPadding() + 24.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp),
        ) {
            item { Statuskarte(laeuft, stand, fehler ?: meldung, ::starten) { BondrDienst.stoppen(context) } }
            stand?.let { s ->
                if (s.wege.isNotEmpty()) item { Ueberschrift("Netze") }
                s.wege.forEach { w ->
                    item(key = w.name) {
                        Netzkarte(w, s.mbitRein, verbrauch[w.name],
                            rolle = { senden("/regel", JSONObject().put("name", w.name).put("gewicht", w.gewicht).put("rolle", it)) },
                            gewicht = { senden("/regel", JSONObject().put("name", w.name).put("gewicht", it).put("rolle", w.rolle)) })
                    }
                }
            }
            if (verbrauch.isNotEmpty()) {
                item { Ueberschrift("Datenverbrauch") }
                item { Verbrauchskarte(verbrauch, laeuft) { zuruecksetzen = true } }
            }
            stand?.let { s ->
                item { Ueberschrift("Verteilung") }
                item { Strategiekarte(s.strategie) { senden("/strategie", JSONObject().put("strategie", it)) } }
            }
            item { Fusszeile(stand?.version) }
        }
    }

    if (zuruecksetzen) {
        AlertDialog(
            onDismissRequest = { zuruecksetzen = false },
            title = { Text("Verbrauch zurücksetzen?") },
            text = { Text("Die Zähler aller Netze beginnen wieder bei null, für den Monat und insgesamt.") },
            confirmButton = {
                TextButton(onClick = {
                    zuruecksetzen = false
                    senden("/verbrauch/zuruecksetzen", JSONObject().put("name", ""))
                }) { Text("Zurücksetzen") }
            },
            dismissButton = { TextButton(onClick = { zuruecksetzen = false }) { Text("Abbrechen") } },
        )
    }
}

@Composable
fun Logo(modifier: Modifier) {
    Box(modifier.clip(RoundedCornerShape(8.dp)).background(Color(0xFF111111))) {
        Canvas(Modifier.fillMaxSize()) {
            val f = size.width / 64f
            val s = Stroke(width = 4.5f * f, cap = StrokeCap.Round)
            val p = androidx.compose.ui.graphics.Path().apply {
                moveTo(12 * f, 18 * f); cubicTo(26 * f, 18 * f, 28 * f, 32 * f, 36 * f, 32 * f)
                moveTo(12 * f, 46 * f); cubicTo(26 * f, 46 * f, 28 * f, 32 * f, 36 * f, 32 * f)
                moveTo(36 * f, 32 * f); lineTo(52 * f, 32 * f)
            }
            drawPath(p, Color.White, style = s)
            drawCircle(Color.White, 4 * f, Offset(12 * f, 18 * f))
            drawCircle(Color.White, 4 * f, Offset(12 * f, 46 * f))
        }
    }
}

@Composable
fun Karte(inhalt: @Composable () -> Unit) {
    Card(
        Modifier.fillMaxWidth().animateContentSize(),
        shape = RoundedCornerShape(16.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
        border = androidx.compose.foundation.BorderStroke(1.dp, MaterialTheme.colorScheme.outlineVariant),
    ) { Column(Modifier.padding(18.dp)) { inhalt() } }
}

@Composable
fun Ueberschrift(t: String) {
    Text(t.uppercase(), Modifier.padding(start = 4.dp, top = 4.dp), style = MaterialTheme.typography.labelMedium,
        color = MaterialTheme.colorScheme.onSurfaceVariant, letterSpacing = 1.sp)
}

@Composable
fun Punkt(farbe: Color) {
    Box(Modifier.size(8.dp).clip(CircleShape).background(farbe))
}

@Composable
fun Statuskarte(laeuft: Boolean, s: Stand?, meldung: String?, starten: () -> Unit, stoppen: () -> Unit) {
    Karte {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Punkt(if (laeuft) Gut else MaterialTheme.colorScheme.outlineVariant)
            Spacer(Modifier.width(8.dp))
            val aktiv = s?.wege?.count { it.rolle != "aus" && !it.gestoert } ?: 0
            Text(
                when {
                    !laeuft -> "Aus"
                    s == null -> "Startet …"
                    else -> "Aktiv · $aktiv ${if (aktiv == 1) "Netz" else "Netze"}"
                },
                style = MaterialTheme.typography.titleSmall, fontWeight = FontWeight.Medium,
            )
        }
        Spacer(Modifier.height(14.dp))
        Text(Format.mbit(s?.mbitRein ?: 0.0), style = MaterialTheme.typography.displaySmall,
            fontWeight = FontWeight.SemiBold, fontFamily = FontFamily.Default)
        Text("Empfang gesamt · Senden " + Format.mbit(s?.mbitRaus ?: 0.0),
            style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
        if (s != null && s.wege.isNotEmpty()) {
            Spacer(Modifier.height(16.dp))
            Anteilsbalken(s.wege)
        }
        if (!laeuft) {
            Spacer(Modifier.height(12.dp))
            Text("Bondr nutzt WLAN und Mobilfunk gleichzeitig: Jede neue Verbindung geht über das Netz, das gerade am wenigsten zu tun hat.",
                style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
        meldung?.let {
            Spacer(Modifier.height(12.dp))
            Text(it, style = MaterialTheme.typography.bodySmall, color = Schlecht)
        }
        Spacer(Modifier.height(18.dp))
        if (laeuft) {
            OutlinedButton(onClick = stoppen, Modifier.fillMaxWidth().height(48.dp), shape = RoundedCornerShape(12.dp)) {
                Text("Stoppen")
            }
        } else {
            Button(onClick = starten, Modifier.fillMaxWidth().height(48.dp), shape = RoundedCornerShape(12.dp),
                colors = ButtonDefaults.buttonColors(containerColor = MaterialTheme.colorScheme.primary)) {
                Text("Starten")
            }
        }
    }
}

@Composable
fun Anteilsbalken(wege: List<Weg>) {
    val summe = wege.sumOf { it.mbitRein }
    val leer = MaterialTheme.colorScheme.surfaceVariant
    Canvas(Modifier.fillMaxWidth().height(8.dp).clip(RoundedCornerShape(4.dp))) {
        drawRect(leer)
        if (summe <= 0.01) return@Canvas
        var x = 0f
        wege.forEach { w ->
            val b = (w.mbitRein / summe * size.width).toFloat()
            drawRect(farbe(w.art), Offset(x, 0f), Size(b, size.height))
            x += b
        }
    }
    Spacer(Modifier.height(8.dp))
    Row(horizontalArrangement = Arrangement.spacedBy(16.dp)) {
        wege.forEach { w ->
            Row(verticalAlignment = Alignment.CenterVertically) {
                Punkt(farbe(w.art))
                Spacer(Modifier.width(6.dp))
                val anteil = if (summe > 0.01) (w.mbitRein / summe * 100).toInt() else 0
                Text("${w.name} $anteil %", style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun Netzkarte(w: Weg, gesamt: Double, v: Verbrauch?, rolle: (String) -> Unit, gewicht: (Int) -> Unit) {
    Karte {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Box(Modifier.size(40.dp).clip(RoundedCornerShape(12.dp)).background(farbe(w.art).copy(alpha = 0.12f)),
                contentAlignment = Alignment.Center) {
                Icon(symbol(w.art), null, tint = farbe(w.art))
            }
            Spacer(Modifier.width(12.dp))
            Column(Modifier.weight(1f)) {
                Text(w.name, style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.SemiBold)
                val (text, f) = when {
                    w.rolle == "aus" -> "Aus" to MaterialTheme.colorScheme.onSurfaceVariant
                    w.gestoert -> "Gestört" to Schlecht
                    w.benutzt -> "Aktiv" to Gut
                    else -> "Bereit" to Warnung
                }
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Punkt(f)
                    Spacer(Modifier.width(6.dp))
                    Text("$text · ${w.offen} offen · ${if (w.ip == "0.0.0.0") w.geraet else w.ip}", style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1, overflow = TextOverflow.Ellipsis)
                }
            }
            Column(horizontalAlignment = Alignment.End) {
                Text(Format.mbit(w.mbitRein), style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.SemiBold)
                Text("↑ " + Format.mbit(w.mbitRaus), style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
        }
        if (w.gestoert && w.fehler != null) {
            Spacer(Modifier.height(8.dp))
            Text(w.fehler, style = MaterialTheme.typography.bodySmall, color = Schlecht, maxLines = 2)
        }
        Spacer(Modifier.height(14.dp))
        Row(horizontalArrangement = Arrangement.spacedBy(10.dp)) {
            Kennzahl("Dieser Monat", Format.menge(v?.monatBytes ?: 0), Modifier.weight(1f))
            Kennzahl("Insgesamt", Format.menge(v?.gesamtBytes ?: 0), Modifier.weight(1f))
        }
        Spacer(Modifier.height(14.dp))
        HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant)
        Spacer(Modifier.height(14.dp))
        val rollen = listOf("haupt" to "Haupt", "reserve" to "Reserve", "aus" to "Aus")
        SingleChoiceSegmentedButtonRow(Modifier.fillMaxWidth()) {
            rollen.forEachIndexed { i, (k, t) ->
                SegmentedButton(selected = w.rolle == k, onClick = { rolle(k) },
                    shape = SegmentedButtonDefaults.itemShape(i, rollen.size)) { Text(t) }
            }
        }
        if (w.rolle != "aus") {
            Spacer(Modifier.height(10.dp))
            var wert by remember(w.gewicht) { mutableFloatStateOf(w.gewicht.toFloat()) }
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text("Gewicht", style = MaterialTheme.typography.bodyMedium, modifier = Modifier.width(72.dp))
                Slider(value = wert, onValueChange = { wert = it }, valueRange = 1f..10f, steps = 8,
                    onValueChangeFinished = { gewicht(wert.toInt()) }, modifier = Modifier.weight(1f))
                Text(wert.toInt().toString(), style = MaterialTheme.typography.titleSmall,
                    modifier = Modifier.width(28.dp).padding(start = 8.dp))
            }
        }
    }
}

@Composable
fun Kennzahl(titel: String, wert: String, modifier: Modifier) {
    Column(modifier.clip(RoundedCornerShape(12.dp)).background(MaterialTheme.colorScheme.surfaceVariant).padding(12.dp)) {
        Text(titel, style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
        Spacer(Modifier.height(2.dp))
        Text(wert, style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.SemiBold)
    }
}

@Composable
fun Verbrauchskarte(v: Map<String, Verbrauch>, laeuft: Boolean, zuruecksetzen: () -> Unit) {
    Karte {
        val monat = v.values.firstOrNull()?.monat ?: ""
        val summe = v.values.sumOf { it.monatBytes }
        Text(Format.monat(monat), style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
        Text(Format.menge(summe), style = MaterialTheme.typography.headlineMedium, fontWeight = FontWeight.SemiBold)
        Spacer(Modifier.height(14.dp))
        v.entries.sortedBy { it.key }.forEach { (name, x) ->
            val art = if (name == "WLAN") "WLAN" else "Handy"
            Row(verticalAlignment = Alignment.CenterVertically) {
                Punkt(farbe(art))
                Spacer(Modifier.width(8.dp))
                Text(name, style = MaterialTheme.typography.bodyMedium, modifier = Modifier.weight(1f))
                Text(Format.menge(x.monatBytes), style = MaterialTheme.typography.bodyMedium, fontWeight = FontWeight.Medium)
            }
            Spacer(Modifier.height(6.dp))
            val anteil = if (summe > 0) x.monatBytes.toFloat() / summe else 0f
            val leer = MaterialTheme.colorScheme.surfaceVariant
            Canvas(Modifier.fillMaxWidth().height(6.dp)) {
                drawRoundRect(leer, cornerRadius = CornerRadius(3.dp.toPx()))
                drawRoundRect(farbe(art), size = Size(size.width * anteil, size.height), cornerRadius = CornerRadius(3.dp.toPx()))
            }
            Spacer(Modifier.height(12.dp))
        }
        val seit = v.values.map { it.seit }.filter { it.isNotEmpty() }.minOrNull()
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text(seit?.let { "Gezählt seit ${Format.datum(it)}" } ?: "", style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.weight(1f))
            if (laeuft) TextButton(onClick = zuruecksetzen) { Text("Zurücksetzen") }
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun Strategiekarte(strategie: String, waehlen: (String) -> Unit) {
    val l = listOf(
        Triple("wenigste", "Ausgleich", "Jede neue Verbindung geht an das Netz mit den wenigsten offenen Verbindungen im Verhältnis zu seinem Gewicht."),
        Triple("reihum", "Reihum", "Neue Verbindungen gehen abwechselnd über die Netze, jedes so oft, wie sein Gewicht sagt."),
        Triple("ausfall", "Ausfall", "Alles läuft über das Hauptnetz mit dem höchsten Gewicht, die anderen springen nur ein, wenn es ausfällt."),
    )
    Karte {
        SingleChoiceSegmentedButtonRow(Modifier.fillMaxWidth()) {
            l.forEachIndexed { i, (k, t, _) ->
                SegmentedButton(selected = strategie == k, onClick = { waehlen(k) },
                    shape = SegmentedButtonDefaults.itemShape(i, l.size)) { Text(t) }
            }
        }
        Spacer(Modifier.height(12.dp))
        Text(l.firstOrNull { it.first == strategie }?.third ?: "", style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant)
    }
}

@Composable
fun Fusszeile(version: String?) {
    Column(Modifier.fillMaxWidth().padding(top = 8.dp, start = 4.dp, end = 4.dp)) {
        Text("Eine einzelne Verbindung läuft immer über ein Netz. Viele gleichzeitige Verbindungen, etwa beim Laden einer Webseite, in App-Stores oder bei Downloads mit mehreren Verbindungen, verteilen sich und nutzen beide Netze zusammen.",
            style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
        Spacer(Modifier.height(8.dp))
        Text("Bondr ${version ?: ""}".trim(), style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant)
    }
}
