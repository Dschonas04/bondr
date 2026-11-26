package io.github.dschonas04.bondr

import android.content.Context
import android.net.ConnectivityManager
import android.net.LinkProperties
import android.net.Network
import android.net.NetworkCapabilities
import android.net.NetworkRequest
import java.net.Inet4Address
import java.net.Inet6Address
import org.json.JSONArray
import org.json.JSONObject

/**
 * Hält WLAN und Mobilfunk gleichzeitig offen und meldet jede Änderung. Android schaltet
 * Mobilfunk sonst ab, sobald WLAN da ist; die Anfrage per requestNetwork hält ihn aktiv.
 */
class Netze(context: Context, private val geaendert: (List<Netz>) -> Unit) {

    data class Netz(
        val network: Network,
        val art: String, // "WLAN" oder "Handy" (wie im Kern)
        val name: String, // "WLAN" oder "Mobilfunk", Schlüssel für Regeln und Verbrauch
        val geraet: String,
        val ip: String,
        val gateway: String?,
        val dns: List<String>,
        val ipv6: Boolean,
    ) {
        val kennung: Long get() = network.toString().toLong()

        fun alsJson(): JSONObject = JSONObject().apply {
            put("schluessel", "$name|$kennung")
            put("name", name)
            put("beschreibung", geraet)
            put("art", art)
            put("index", kennung)
            put("ip", ip)
            gateway?.let { put("gateway", it) }
            put("dns", JSONArray(dns))
            put("ipv6", ipv6)
        }
    }

    private val cm = context.getSystemService(ConnectivityManager::class.java)
    private val netze = linkedMapOf<Network, Netz>()
    private val rueckrufe = mutableListOf<ConnectivityManager.NetworkCallback>()

    fun starten() {
        anfordern(NetworkCapabilities.TRANSPORT_WIFI, "WLAN", "WLAN")
        anfordern(NetworkCapabilities.TRANSPORT_CELLULAR, "Handy", "Mobilfunk")
    }

    fun beenden() {
        rueckrufe.forEach { runCatching { cm.unregisterNetworkCallback(it) } }
        rueckrufe.clear()
        synchronized(netze) { netze.clear() }
    }

    fun suchen(kennung: Long): Network? = synchronized(netze) {
        netze.keys.firstOrNull { it.toString().toLong() == kennung }
    }

    fun alle(): List<Netz> = synchronized(netze) { netze.values.toList() }

    private fun anfordern(transport: Int, art: String, name: String) {
        val anfrage = NetworkRequest.Builder()
            .addTransportType(transport)
            .addCapability(NetworkCapabilities.NET_CAPABILITY_INTERNET)
            .build()
        val r = object : ConnectivityManager.NetworkCallback() {
            override fun onLinkPropertiesChanged(network: Network, lp: LinkProperties) {
                val neu = umwandeln(network, lp, art, name)
                synchronized(netze) {
                    if (neu == null) netze.remove(network) else netze[network] = neu
                }
                geaendert(alle())
            }

            override fun onLost(network: Network) {
                synchronized(netze) { netze.remove(network) }
                geaendert(alle())
            }
        }
        cm.requestNetwork(anfrage, r)
        rueckrufe += r
    }

    private fun umwandeln(network: Network, lp: LinkProperties, art: String, name: String): Netz? {
        val adressen = lp.linkAddresses.map { it.address }
        // Mobilfunk oft nur mit IPv6 (Telekom: IPv4 über 464XLAT): dann 0.0.0.0, das Netz
        // wählt die Adresse selbst
        val ip = adressen.firstOrNull { it is Inet4Address }?.hostAddress
            ?: if (adressen.any { it is Inet6Address && !it.isLinkLocalAddress }) "0.0.0.0" else return null
        val gw = lp.routes.firstOrNull { it.isDefaultRoute && it.gateway is Inet4Address }?.gateway?.hostAddress
        val v4 = lp.dnsServers.filterIsInstance<Inet4Address>().mapNotNull { it.hostAddress }
        val v6 = lp.dnsServers.filterIsInstance<Inet6Address>().mapNotNull { it.hostAddress?.substringBefore('%') }
        val dns = v4.ifEmpty { listOf("1.1.1.1", "8.8.8.8") } + v6
        val geraet = (lp.interfaceName ?: "") + if (ip == "0.0.0.0") " · IPv6 mit 464XLAT" else ""
        val hatV6 = adressen.any { it is Inet6Address && !it.isLinkLocalAddress && !it.isSiteLocalAddress &&
            (it.address[0].toInt() and 0xfe) != 0xfc } &&
            lp.routes.any { it.isDefaultRoute && it.destination.address is Inet6Address }
        return Netz(network, art, name, geraet, ip, gw, dns, hatV6)
    }

    companion object {
        fun alsJson(l: List<Netz>): String = JSONArray(l.map { it.alsJson() }).toString()
    }
}
