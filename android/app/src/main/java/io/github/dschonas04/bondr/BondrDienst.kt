package io.github.dschonas04.bondr

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.content.pm.ServiceInfo
import android.net.VpnService
import android.os.Build
import android.os.ParcelFileDescriptor
import android.service.quicksettings.TileService
import android.content.ComponentName
import android.util.Log
import kotlin.concurrent.thread
import kotlinx.coroutines.flow.MutableStateFlow

/**
 * Der VPN-Dienst: hält WLAN und Mobilfunk offen, öffnet den Tunnel und startet den Kern,
 * der jede Verbindung aus dem Tunnel über eines der Netze schickt.
 */
class BondrDienst : VpnService() {

    private lateinit var netze: Netze
    private lateinit var kern: Kern
    private var appSocket: AppSocket? = null
    private var tunnel: ParcelFileDescriptor? = null
    @Volatile private var aktiv = false

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        if (intent?.action == AKTION_STOPP) {
            stoppen()
            return START_NOT_STICKY
        }
        if (aktiv) return START_STICKY
        vordergrund()
        aktiv = true
        laeuft.value = true
        kern = Kern(this)
        netze = Netze(this) { l -> melden(l) }
        thread(name = "bondr-start") {
            try {
                // prepare() meldet die App als aktuelles VPN an (null = schon erlaubt)
                if (prepare(this) != null) throw IllegalStateException("Bondr darf noch kein VPN öffnen. Bitte einmal in der App starten und erlauben.")
                netze.starten()
                tunnel = tunnelOeffnen() ?: throw IllegalStateException("Android hat den Tunnel nicht geöffnet")
                appSocket = AppSocket(netze) { tunnel }.also { it.starten() }
                kern.starten()
                ueberwachen()
            } catch (e: Exception) {
                Log.e("Bondr", "Start", e)
                fehler.value = e.message
                stoppen()
            }
        }
        return START_STICKY
    }

    private fun tunnelOeffnen(): ParcelFileDescriptor? = Builder()
        .setSession("Bondr")
        .setMtu(1500)
        .addAddress("172.19.0.1", 30)
        .addRoute("0.0.0.0", 0)
        // IPv6 kommt in den Tunnel und wird dort abgelehnt, damit die Apps IPv4 nehmen
        .addAddress("fd00:b0d::1", 126)
        .addRoute("::", 0)
        .addDnsServer("172.19.0.2")
        .addDisallowedApplication(packageName)
        .setBlocking(false)
        .setMetered(true)
        .establish()

    /** Meldet die Netze an den Kern und an Android (Anzeige, Abrechnung). */
    private fun melden(l: List<Netze.Netz>) {
        runCatching { setUnderlyingNetworks(l.map { it.network }.toTypedArray()) }
        thread {
            repeat(20) {
                val r = runCatching { kern.anfrage("POST", "/wege", Netze.alsJson(netze.alle())) }
                if (r.isSuccess) return@thread
                Log.w("Bondr", "Netze melden: ${r.exceptionOrNull()?.message}")
                Thread.sleep(500)
            }
        }
    }

    /** Fällt der Kern aus, endet der Dienst, damit kein toter Tunnel den Verkehr schluckt. */
    private fun ueberwachen() {
        melden(netze.alle())
        while (aktiv) {
            Thread.sleep(2000)
            if (aktiv && !kern.laeuft()) {
                fehler.value = "Der Kern ist unerwartet beendet worden."
                stoppen()
            }
        }
    }

    private fun vordergrund() {
        val nm = getSystemService(NotificationManager::class.java)
        nm.createNotificationChannel(NotificationChannel(KANAL, "Bondr aktiv", NotificationManager.IMPORTANCE_LOW).apply {
            description = "Zeigt an, dass Bondr den Verkehr auf WLAN und Mobilfunk verteilt"
            setShowBadge(false)
        })
        val oeffnen = PendingIntent.getActivity(this, 0, Intent(this, MainActivity::class.java),
            PendingIntent.FLAG_IMMUTABLE)
        val stopp = PendingIntent.getService(this, 1, Intent(this, BondrDienst::class.java).setAction(AKTION_STOPP),
            PendingIntent.FLAG_IMMUTABLE)
        val n = Notification.Builder(this, KANAL)
            .setSmallIcon(R.drawable.ic_benachrichtigung)
            .setContentTitle("Bondr ist aktiv")
            .setContentText("Verteilt den Datenverkehr auf WLAN und Mobilfunk")
            .setContentIntent(oeffnen)
            .setOngoing(true)
            .addAction(Notification.Action.Builder(null, "Beenden", stopp).build())
            .build()
        if (Build.VERSION.SDK_INT >= 34) {
            startForeground(1, n, ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE)
        } else {
            startForeground(1, n)
        }
    }

    private fun stoppen() {
        if (!aktiv) {
            stopSelf()
            return
        }
        aktiv = false
        thread {
            runCatching { kern.beenden() }
            appSocket?.beenden()
            appSocket = null
            runCatching { tunnel?.close() }
            tunnel = null
            runCatching { netze.beenden() }
            laeuft.value = false
            TileService.requestListeningState(this, ComponentName(this, Kachel::class.java))
            stopForeground(STOP_FOREGROUND_REMOVE)
            stopSelf()
        }
    }

    override fun onRevoke() {
        stoppen()
    }

    override fun onDestroy() {
        if (aktiv) stoppen()
        super.onDestroy()
    }

    companion object {
        const val AKTION_STOPP = "io.github.dschonas04.bondr.STOPP"
        private const val KANAL = "aktiv"
        val laeuft = MutableStateFlow(false)
        val fehler = MutableStateFlow<String?>(null)

        fun starten(context: Context) {
            fehler.value = null
            context.startForegroundService(Intent(context, BondrDienst::class.java))
        }

        fun stoppen(context: Context) {
            context.startService(Intent(context, BondrDienst::class.java).setAction(AKTION_STOPP))
        }
    }
}
