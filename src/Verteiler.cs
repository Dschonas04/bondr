using System;
using System.Collections.Generic;
using System.IO;
using System.Linq;
using System.Net;
using System.Net.Sockets;
using System.Threading;

namespace Bondr
{
    /// <summary>Ein Weg mit Zählern und Zustand.</summary>
    public sealed class Pfad
    {
        public Weg Weg;
        public long Offen;
        public long Rein;
        public long Raus;
        public long Zuletzt;
        public long GestoertBis;
        public string Fehler;
        internal long letzteRein, letzteRaus;
        public double RateRein, RateRaus, Spitze; // Bytes/s

        public string Name { get { return Weg.Name; } }
        public bool Gestoert { get { return Interlocked.Read(ref GestoertBis) > DateTime.UtcNow.Ticks; } }
    }

    /// <summary>Entscheidet für jede neue Verbindung, über welchen Weg sie läuft.</summary>
    public sealed class Verteiler
    {
        readonly object sperre = new object();
        readonly Dictionary<string, Pfad> pfade = new Dictionary<string, Pfad>();
        readonly HashSet<string> aus;
        readonly Func<List<Weg>> finden;
        public event Action<string[]> AusGeaendert;

        public Verteiler(IEnumerable<string> abgeschaltet, Func<List<Weg>> finden)
        {
            aus = new HashSet<string>(abgeschaltet ?? new string[0]);
            this.finden = finden ?? Weg.Finden;
            Aktualisieren();
        }

        public void Aktualisieren()
        {
            List<Weg> gefunden;
            try { gefunden = finden(); } catch { return; }
            lock (sperre)
            {
                var da = new HashSet<string>();
                foreach (var w in gefunden)
                {
                    da.Add(w.Schluessel);
                    Pfad p;
                    if (pfade.TryGetValue(w.Schluessel, out p)) p.Weg = w;
                    else pfade[w.Schluessel] = new Pfad { Weg = w };
                }
                foreach (var k in pfade.Keys.ToList())
                    if (!da.Contains(k) && Interlocked.Read(ref pfade[k].Offen) == 0) pfade.Remove(k);
            }
        }

        /// <summary>Einmal pro Sekunde aufrufen.</summary>
        public void Messen()
        {
            lock (sperre)
            {
                foreach (var p in pfade.Values)
                {
                    long r = Interlocked.Read(ref p.Rein), s = Interlocked.Read(ref p.Raus);
                    p.RateRein = 0.5 * p.RateRein + 0.5 * (r - p.letzteRein);
                    p.RateRaus = 0.5 * p.RateRaus + 0.5 * (s - p.letzteRaus);
                    p.letzteRein = r;
                    p.letzteRaus = s;
                    p.Spitze *= 0.995;
                    if (p.RateRein > p.Spitze) p.Spitze = p.RateRein;
                }
            }
        }

        public bool IstAn(string name) { lock (sperre) return !aus.Contains(name); }

        public void Schalten(string name, bool an)
        {
            string[] liste;
            lock (sperre)
            {
                if (an) aus.Remove(name); else aus.Add(name);
                liste = aus.OrderBy(x => x).ToArray();
            }
            var h = AusGeaendert;
            if (h != null) h(liste);
        }

        public List<Pfad> Alle()
        {
            lock (sperre) return pfade.Values.OrderBy(p => Weg.Rang(p.Weg.Art)).ThenBy(p => p.Name).ToList();
        }

        /// <summary>Benutzbare Wege, der günstigste zuerst: wenig offen im Verhältnis zur Leistung.</summary>
        public List<Pfad> Pfade()
        {
            List<Pfad> an;
            lock (sperre) an = pfade.Values.Where(p => !aus.Contains(p.Name)).ToList();
            var gut = an.Where(p => !p.Gestoert)
                .OrderBy(p => (Interlocked.Read(ref p.Offen) + 1) / (p.Spitze + 1e6))
                .ThenBy(p => Interlocked.Read(ref p.Zuletzt)).ToList();
            gut.AddRange(an.Where(p => p.Gestoert));
            return gut;
        }

        static IPAddress[] Aufloesen(string host)
        {
            IPAddress ip;
            if (IPAddress.TryParse(host, out ip)) return new[] { ip };
            return Dns.GetHostAddresses(host).Where(a => a.AddressFamily == AddressFamily.InterNetwork).ToArray();
        }

        /// <summary>Verbindet über den günstigsten Weg und weicht bei Fehlern auf die anderen aus.</summary>
        public Stream Verbinden(string host, int port, out Pfad gewaehlt)
        {
            var liste = Pfade();
            if (liste.Count == 0) throw new IOException("kein Weg ins Internet");
            Exception letzter = null;
            foreach (var p in liste)
            {
                try
                {
                    var s = VerbindenUeber(p, host, port);
                    Interlocked.Exchange(ref p.GestoertBis, 0);
                    gewaehlt = p;
                    return s;
                }
                catch (Exception e)
                {
                    letzter = e;
                    Interlocked.Exchange(ref p.GestoertBis, DateTime.UtcNow.AddSeconds(20).Ticks);
                    p.Fehler = e.Message;
                }
            }
            throw new IOException(letzter != null ? letzter.Message : "keine Verbindung", letzter);
        }

        public Stream VerbindenUeber(Pfad p, string host, int port)
        {
            Exception letzter = null;
            var ips = Aufloesen(host);
            if (ips.Length == 0) throw new IOException("keine IPv4-Adresse für " + host);
            foreach (var ip in ips)
            {
                try
                {
                    var sock = p.Weg.Verbinden(ip, port, 10000);
                    Interlocked.Increment(ref p.Offen);
                    Interlocked.Exchange(ref p.Zuletzt, DateTime.UtcNow.Ticks);
                    return new Gezaehlt(sock, p);
                }
                catch (Exception e) { letzter = e; }
            }
            throw new IOException(letzter.Message, letzter);
        }
    }

    /// <summary>Netzwerkstrom, der die Bytes seinem Weg zuschreibt.</summary>
    public sealed class Gezaehlt : NetworkStream
    {
        readonly Pfad p;
        int zu;
        public readonly Socket Sock;

        public Gezaehlt(Socket s, Pfad p) : base(s, true)
        {
            this.p = p;
            Sock = s;
        }

        public override int Read(byte[] buffer, int offset, int size)
        {
            int n = base.Read(buffer, offset, size);
            Interlocked.Add(ref p.Rein, n);
            return n;
        }

        public override void Write(byte[] buffer, int offset, int size)
        {
            base.Write(buffer, offset, size);
            Interlocked.Add(ref p.Raus, size);
        }

        public void HalbZu()
        {
            try { Sock.Shutdown(SocketShutdown.Send); } catch { }
        }

        protected override void Dispose(bool disposing)
        {
            if (Interlocked.Exchange(ref zu, 1) == 0) Interlocked.Decrement(ref p.Offen);
            base.Dispose(disposing);
        }
    }
}
