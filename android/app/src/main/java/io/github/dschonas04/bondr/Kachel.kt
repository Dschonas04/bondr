package io.github.dschonas04.bondr

import android.app.PendingIntent
import android.content.Intent
import android.net.VpnService
import android.os.Build
import android.service.quicksettings.Tile
import android.service.quicksettings.TileService

/** Schnelleinstellung: Bondr mit einem Tipp ein- und ausschalten. */
class Kachel : TileService() {
    override fun onStartListening() {
        val t = qsTile ?: return
        t.state = if (BondrDienst.laeuft.value) Tile.STATE_ACTIVE else Tile.STATE_INACTIVE
        t.subtitle = if (BondrDienst.laeuft.value) "WLAN + Mobilfunk" else "Aus"
        t.updateTile()
    }

    override fun onClick() {
        if (BondrDienst.laeuft.value) {
            BondrDienst.stoppen(this)
        } else if (VpnService.prepare(this) == null) {
            BondrDienst.starten(this)
        } else {
            // Erst einmal in der App erlauben lassen
            val i = Intent(this, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
            if (Build.VERSION.SDK_INT >= 34) {
                startActivityAndCollapse(PendingIntent.getActivity(this, 0, i, PendingIntent.FLAG_IMMUTABLE))
            } else {
                @Suppress("DEPRECATION")
                startActivityAndCollapse(i)
            }
        }
        qsTile?.let {
            it.state = if (BondrDienst.laeuft.value) Tile.STATE_INACTIVE else Tile.STATE_ACTIVE
            it.updateTile()
        }
    }
}
