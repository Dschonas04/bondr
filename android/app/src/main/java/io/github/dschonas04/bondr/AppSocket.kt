package io.github.dschonas04.bondr

import android.net.LocalServerSocket
import android.net.LocalSocket
import android.os.ParcelFileDescriptor
import android.os.Process
import android.system.Os
import android.util.Log
import java.io.FileDescriptor
import java.io.IOException
import kotlin.concurrent.thread

/**
 * Socket im abstrakten Namensraum, über den der Go-Kern die App um zwei Dinge bittet:
 * 'B' + Netzkennung + Deskriptor: Socket an dieses Netz binden (Network.bindSocket),
 * 'T': den Deskriptor des VPN-Tunnels schicken. Nur der eigene Benutzer wird bedient.
 */
class AppSocket(
    private val netze: Netze,
    private val tunnel: () -> ParcelFileDescriptor?,
) {
    private var server: LocalServerSocket? = null
    @Volatile private var laeuft = false

    fun starten() {
        val s = LocalServerSocket(NAME)
        server = s
        laeuft = true
        thread(name = "bondr-appsocket", isDaemon = true) {
            while (laeuft) {
                val c = try { s.accept() } catch (e: IOException) { break }
                thread(name = "bondr-appsocket-verbindung", isDaemon = true) { bedienen(c) }
            }
        }
    }

    fun beenden() {
        laeuft = false
        runCatching { server?.close() }
        server = null
    }

    private fun bedienen(c: LocalSocket) {
        c.use {
            if (c.peerCredentials.uid != Process.myUid()) return
            val ein = c.inputStream
            val aus = c.outputStream
            val puffer = ByteArray(5)
            while (laeuft) {
                var n = 0
                while (n < 5) {
                    val r = try { ein.read(puffer, n, 5 - n) } catch (e: IOException) { -1 }
                    if (r < 0) return
                    n += r
                }
                val wert = ((puffer[1].toLong() and 0xff) shl 24) or ((puffer[2].toLong() and 0xff) shl 16) or
                    ((puffer[3].toLong() and 0xff) shl 8) or (puffer[4].toLong() and 0xff)
                when (puffer[0].toInt().toChar()) {
                    'B' -> {
                        val fds = c.ancillaryFileDescriptors
                        val ok = fds?.firstOrNull()?.let { binden(it, wert) } ?: false
                        fds?.forEach { schliessen(it) }
                        aus.write(byteArrayOf(if (ok) 0 else 1))
                    }
                    'T' -> {
                        val t = tunnel()
                        if (t == null) {
                            aus.write(byteArrayOf(2))
                        } else {
                            c.setFileDescriptorsForSend(arrayOf(t.fileDescriptor))
                            aus.write(byteArrayOf(0))
                            c.setFileDescriptorsForSend(null)
                        }
                    }
                    else -> aus.write(byteArrayOf(3))
                }
                aus.flush()
            }
        }
    }

    private fun binden(fd: FileDescriptor, kennung: Long): Boolean {
        val netz = netze.suchen(kennung) ?: return false
        return try {
            netz.bindSocket(fd)
            true
        } catch (e: Exception) {
            Log.w("Bondr", "bindSocket $kennung: ${e.message}")
            false
        }
    }

    private fun schliessen(fd: FileDescriptor) {
        runCatching { Os.close(fd) }
    }

    companion object {
        const val NAME = "bondr-app"
    }
}
