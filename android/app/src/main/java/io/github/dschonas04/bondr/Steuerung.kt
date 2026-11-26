package io.github.dschonas04.bondr

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent

/**
 * Steuerung per adb, ohne das Handy zu entsperren:
 *   adb shell am broadcast -a io.github.dschonas04.bondr.START -n io.github.dschonas04.bondr/.Steuerung
 * Geschützt mit DUMP, das haben nur die Shell und das System.
 */
class Steuerung : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        when (intent.action) {
            "io.github.dschonas04.bondr.START" -> BondrDienst.starten(context)
            "io.github.dschonas04.bondr.STOPP" -> BondrDienst.stoppen(context)
        }
    }
}
