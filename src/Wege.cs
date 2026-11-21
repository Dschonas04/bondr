using System;
using System.Collections.Generic;
using System.Linq;
using System.Net;
using System.Net.NetworkInformation;
using System.Net.Sockets;

namespace Bondr
{
    /// <summary>Ein Adapter mit eigener IPv4-Adresse und eigenem Gateway.</summary>
    public sealed class Weg
    {
        public string Schluessel;
        public string Name;
        public string Beschreibung;
        public string Art;
        public int Index;
        public IPAddress IP;
        public IPAddress Gateway;

        /// <summary>Alle Adapter, die verbunden sind und ein IPv4-Gateway haben.</summary>
        public static List<Weg> Finden()
        {
            var liste = new List<Weg>();
            NetworkInterface[] alle;
            try { alle = NetworkInterface.GetAllNetworkInterfaces(); }
            catch { return liste; }
            foreach (var ni in alle)
            {
                if (ni.OperationalStatus != OperationalStatus.Up) continue;
                if (ni.NetworkInterfaceType == NetworkInterfaceType.Loopback || ni.NetworkInterfaceType == NetworkInterfaceType.Tunnel) continue;
                IPInterfaceProperties eig;
                IPv4InterfaceProperties v4;
                try
                {
                    eig = ni.GetIPProperties();
                    v4 = eig.GetIPv4Properties();
                }
                catch { continue; }
                if (v4 == null) continue;
                var gw = eig.GatewayAddresses.Select(g => g.Address)
                    .FirstOrDefault(a => a.AddressFamily == AddressFamily.InterNetwork && !a.Equals(IPAddress.Any));
                var ip = eig.UnicastAddresses.Select(u => u.Address)
                    .FirstOrDefault(a => a.AddressFamily == AddressFamily.InterNetwork && !a.ToString().StartsWith("169.254."));
                if (gw == null || ip == null) continue;
                liste.Add(new Weg
                {
                    Schluessel = ni.Name + "|" + ip,
                    Name = ni.Name,
                    Beschreibung = ni.Description,
                    Art = ArtAus(ni),
                    Index = v4.Index,
                    IP = ip,
                    Gateway = gw
                });
            }
            return liste.OrderBy(w => Rang(w.Art)).ThenBy(w => w.Name).ToList();
        }

        public static int Rang(string art)
        {
            switch (art)
            {
                case "Kabel": return 0;
                case "WLAN": return 1;
                case "Handy": return 2;
                default: return 3;
            }
        }

        static string ArtAus(NetworkInterface ni)
        {
            var t = ni.Name + " " + ni.Description;
            if (Enthaelt(t, "NDIS", "Android", "iPhone", "Apple Mobile", "Samsung", "Mobile", "Tethering")) return "Handy";
            if (ni.NetworkInterfaceType == NetworkInterfaceType.Wireless80211 || Enthaelt(t, "WLAN", "Wi-Fi", "Wireless")) return "WLAN";
            if (ni.NetworkInterfaceType == NetworkInterfaceType.Ethernet || ni.NetworkInterfaceType == NetworkInterfaceType.GigabitEthernet) return "Kabel";
            return "Sonstiges";
        }

        static bool Enthaelt(string s, params string[] teile)
        {
            return teile.Any(t => s.IndexOf(t, StringComparison.OrdinalIgnoreCase) >= 0);
        }

        const int IP_UNICAST_IF = 31;

        /// <summary>
        /// Baut eine TCP-Verbindung, die fest über diesen Adapter läuft: an seine Adresse
        /// gebunden und mit IP_UNICAST_IF, damit Windows nicht doch die Hauptroute nimmt.
        /// </summary>
        public Socket Verbinden(IPAddress ziel, int port, int zeitMs)
        {
            var s = new Socket(AddressFamily.InterNetwork, SocketType.Stream, ProtocolType.Tcp);
            try
            {
                s.NoDelay = true;
                // Index in Netzwerk-Byte-Reihenfolge
                s.SetSocketOption(SocketOptionLevel.IP, (SocketOptionName)IP_UNICAST_IF,
                    BitConverter.GetBytes(IPAddress.HostToNetworkOrder(Index)));
                s.Bind(new IPEndPoint(IP, 0));
                var ar = s.BeginConnect(ziel, port, null, null);
                if (!ar.AsyncWaitHandle.WaitOne(zeitMs))
                    throw new TimeoutException("keine Antwort von " + ziel + ":" + port);
                s.EndConnect(ar);
                return s;
            }
            catch
            {
                s.Close();
                throw;
            }
        }
    }
}
