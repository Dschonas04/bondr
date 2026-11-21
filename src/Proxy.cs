using System;
using System.Collections.Generic;
using System.IO;
using System.Net;
using System.Net.Sockets;
using System.Text;
using System.Threading;

namespace Bondr
{
    /// <summary>SOCKS5- und HTTP-Proxy auf einem Port; jede Verbindung geht an den Verteiler.</summary>
    public sealed class Proxy
    {
        readonly Verteiler v;
        readonly TcpListener lauscher;
        volatile bool laeuft;

        public Proxy(Verteiler v, int port)
        {
            this.v = v;
            lauscher = new TcpListener(IPAddress.Loopback, port);
        }

        public void Starten()
        {
            lauscher.Start(512);
            laeuft = true;
            var t = new Thread(Annehmen) { IsBackground = true, Name = "Proxy" };
            t.Start();
        }

        public void Stoppen()
        {
            laeuft = false;
            try { lauscher.Stop(); } catch { }
        }

        void Annehmen()
        {
            while (laeuft)
            {
                TcpClient c;
                try { c = lauscher.AcceptTcpClient(); }
                catch { if (!laeuft) return; continue; }
                ThreadPool.QueueUserWorkItem(_ => Bedienen(c));
            }
        }

        void Bedienen(TcpClient c)
        {
            try
            {
                c.NoDelay = true;
                using (c)
                using (var s = c.GetStream())
                {
                    s.ReadTimeout = 30000;
                    var l = new Leser(s);
                    int erstes = l.Spitze();
                    if (erstes < 0) return;
                    if (erstes == 5) Socks(c, s, l);
                    else Http(c, s, l);
                }
            }
            catch { }
        }

        // RFC 1928, nur CONNECT ohne Anmeldung
        void Socks(TcpClient c, NetworkStream s, Leser l)
        {
            var kopf = l.Bytes(2);
            l.Bytes(kopf[1]);
            s.Write(new byte[] { 5, 0 }, 0, 2);
            var a = l.Bytes(4);
            if (a[0] != 5) return;
            if (a[1] != 1)
            {
                s.Write(new byte[] { 5, 7, 0, 1, 0, 0, 0, 0, 0, 0 }, 0, 10);
                return;
            }
            string host;
            switch (a[3])
            {
                case 1: host = new IPAddress(l.Bytes(4)).ToString(); break;
                case 3: host = Encoding.ASCII.GetString(l.Bytes(l.Bytes(1)[0])); break;
                case 4: host = new IPAddress(l.Bytes(16)).ToString(); break;
                default:
                    s.Write(new byte[] { 5, 8, 0, 1, 0, 0, 0, 0, 0, 0 }, 0, 10);
                    return;
            }
            var pb = l.Bytes(2);
            int port = (pb[0] << 8) | pb[1];
            Stream z;
            Pfad p;
            try { z = v.Verbinden(host, port, out p); }
            catch
            {
                s.Write(new byte[] { 5, 5, 0, 1, 0, 0, 0, 0, 0, 0 }, 0, 10);
                return;
            }
            using (z)
            {
                s.Write(new byte[] { 5, 0, 0, 1, 0, 0, 0, 0, 0, 0 }, 0, 10);
                Durchreichen(c, s, l, z);
            }
        }

        void Http(TcpClient c, NetworkStream s, Leser l)
        {
            var zeile = l.Zeile();
            if (string.IsNullOrEmpty(zeile)) return;
            var teile = zeile.Split(' ');
            if (teile.Length < 3) return;
            var kopfzeilen = new List<string>();
            string z2;
            while (!string.IsNullOrEmpty(z2 = l.Zeile())) kopfzeilen.Add(z2);

            string host;
            int port;
            if (teile[0] == "CONNECT")
            {
                Trennen(teile[1], 443, out host, out port);
                Stream z;
                Pfad p;
                try { z = v.Verbinden(host, port, out p); }
                catch { Antwort(s, "502 Bad Gateway"); return; }
                using (z)
                {
                    var ok = Encoding.ASCII.GetBytes("HTTP/1.1 200 Connection established\r\n\r\n");
                    s.Write(ok, 0, ok.Length);
                    Durchreichen(c, s, l, z);
                }
                return;
            }

            // Unverschlüsseltes HTTP: eine Anfrage je Verbindung, danach zu.
            Uri u;
            if (!Uri.TryCreate(teile[1], UriKind.Absolute, out u) || u.Scheme != "http")
            {
                Antwort(s, "400 Bad Request");
                return;
            }
            host = u.Host;
            port = u.Port;
            var sb = new StringBuilder();
            sb.Append(teile[0]).Append(' ').Append(u.PathAndQuery).Append(' ').Append(teile[2]).Append("\r\n");
            foreach (var k in kopfzeilen)
            {
                var n = k.Split(':')[0].Trim().ToLowerInvariant();
                if (n == "proxy-connection" || n == "proxy-authorization" || n == "connection" || n == "keep-alive") continue;
                sb.Append(k).Append("\r\n");
            }
            sb.Append("Connection: close\r\n\r\n");
            Stream ziel;
            Pfad gp;
            try { ziel = v.Verbinden(host, port, out gp); }
            catch { Antwort(s, "502 Bad Gateway"); return; }
            using (ziel)
            {
                var b = Encoding.ASCII.GetBytes(sb.ToString());
                ziel.Write(b, 0, b.Length);
                Durchreichen(c, s, l, ziel);
            }
        }

        static void Trennen(string hp, int standard, out string host, out int port)
        {
            port = standard;
            host = hp;
            if (hp.StartsWith("["))
            {
                int e = hp.IndexOf(']');
                host = hp.Substring(1, e - 1);
                if (e + 2 < hp.Length) int.TryParse(hp.Substring(e + 2), out port);
                return;
            }
            int i = hp.LastIndexOf(':');
            if (i > 0)
            {
                host = hp.Substring(0, i);
                int.TryParse(hp.Substring(i + 1), out port);
            }
        }

        static void Antwort(Stream s, string status)
        {
            var b = Encoding.ASCII.GetBytes("HTTP/1.1 " + status + "\r\nContent-Length: 0\r\nConnection: close\r\n\r\n");
            s.Write(b, 0, b.Length);
        }

        /// <summary>Kopiert in beide Richtungen, bis beide Seiten fertig sind.</summary>
        static void Durchreichen(TcpClient c, NetworkStream s, Leser l, Stream z)
        {
            s.ReadTimeout = Timeout.Infinite;
            var rest = l.Rest();
            if (rest.Length > 0) z.Write(rest, 0, rest.Length);
            var hin = new Thread(() =>
            {
                try { Kopieren(s, z); } catch { }
                var g = z as Gezaehlt;
                if (g != null) g.HalbZu();
            }) { IsBackground = true };
            hin.Start();
            try { Kopieren(z, s); } catch { }
            try { c.Client.Shutdown(SocketShutdown.Send); } catch { }
            hin.Join(TimeSpan.FromSeconds(30));
        }

        static void Kopieren(Stream von, Stream nach)
        {
            var b = new byte[64 * 1024];
            int n;
            while ((n = von.Read(b, 0, b.Length)) > 0) nach.Write(b, 0, n);
        }
    }

    /// <summary>Liest gepuffert Bytes und Zeilen und gibt den Rest an den Tunnel weiter.</summary>
    sealed class Leser
    {
        readonly Stream s;
        readonly byte[] puf = new byte[16 * 1024];
        int anf, end;

        public Leser(Stream s) { this.s = s; }

        bool Fuellen()
        {
            if (anf > 0 && anf == end) { anf = end = 0; }
            if (end == puf.Length)
            {
                if (anf == 0) throw new IOException("Kopf zu lang");
                Buffer.BlockCopy(puf, anf, puf, 0, end - anf);
                end -= anf;
                anf = 0;
            }
            int n = s.Read(puf, end, puf.Length - end);
            if (n <= 0) return false;
            end += n;
            return true;
        }

        public int Spitze()
        {
            if (anf == end && !Fuellen()) return -1;
            return puf[anf];
        }

        public byte[] Bytes(int n)
        {
            var b = new byte[n];
            int o = 0;
            while (o < n)
            {
                if (anf == end && !Fuellen()) throw new EndOfStreamException();
                int k = Math.Min(n - o, end - anf);
                Buffer.BlockCopy(puf, anf, b, o, k);
                anf += k;
                o += k;
            }
            return b;
        }

        public string Zeile()
        {
            var sb = new StringBuilder();
            while (true)
            {
                if (anf == end && !Fuellen()) return sb.Length > 0 ? sb.ToString() : null;
                byte c = puf[anf++];
                if (c == '\n') break;
                if (c != '\r') sb.Append((char)c);
                if (sb.Length > 16384) throw new IOException("Zeile zu lang");
            }
            return sb.ToString();
        }

        public byte[] Rest()
        {
            var b = new byte[end - anf];
            Buffer.BlockCopy(puf, anf, b, 0, b.Length);
            anf = end;
            return b;
        }
    }
}
