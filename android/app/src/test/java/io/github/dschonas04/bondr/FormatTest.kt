package io.github.dschonas04.bondr

import org.junit.Assert.assertEquals
import org.junit.Test

class FormatTest {
    @Test fun menge() {
        assertEquals("1,24 GB", Format.menge(1_240_000_000L))
        assertEquals("512 MB", Format.menge(512_000_000L))
        assertEquals("12 KB", Format.menge(12_000L))
    }

    @Test fun mbit() {
        assertEquals("48,2 Mbit/s", Format.mbit(48.21))
        assertEquals("312 Mbit/s", Format.mbit(312.4))
    }

    @Test fun datum() {
        assertEquals("03.11.2026", Format.datum("2026-11-03"))
        assertEquals("November 2026", Format.monat("2026-11"))
    }
}
