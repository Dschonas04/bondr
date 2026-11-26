package io.github.dschonas04.bondr

import java.text.NumberFormat
import java.util.Locale

object Format {
    private fun zahl(x: Double, stellen: Int): String =
        NumberFormat.getNumberInstance(Locale.GERMANY).apply {
            minimumFractionDigits = stellen
            maximumFractionDigits = stellen
        }.format(x)

    fun mbit(x: Double): String = zahl(x, if (x >= 100) 0 else 1) + " Mbit/s"

    /** Datenmenge in Bytes, ab 1 GB in GB mit zwei Stellen. */
    fun menge(bytes: Long): String = when {
        bytes >= 1_000_000_000L -> zahl(bytes / 1e9, 2) + " GB"
        bytes >= 1_000_000L -> zahl(bytes / 1e6, 0) + " MB"
        else -> zahl(bytes / 1e3, 0) + " KB"
    }

    /** 2026-11-03 → 03.11.2026 */
    fun datum(iso: String): String {
        val t = iso.split("-")
        return if (t.size == 3) "${t[2]}.${t[1]}.${t[0]}" else iso
    }

    private val monate = listOf("Januar", "Februar", "März", "April", "Mai", "Juni", "Juli", "August",
        "September", "Oktober", "November", "Dezember")

    /** 2026-11 → November 2026 */
    fun monat(iso: String): String {
        val t = iso.split("-")
        val m = t.getOrNull(1)?.toIntOrNull() ?: return iso
        return "${monate.getOrElse(m - 1) { iso }} ${t[0]}"
    }
}
