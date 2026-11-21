using System;
using System.Collections.Generic;
using System.IO;
using System.Linq;
using System.Text;
using System.Threading;

namespace Bondr
{
    /// <summary>Ein laufender oder fertiger Download.</summary>
    public sealed class Auftrag
    {
        public int ID;
        public string Url;
        public string Datei = "";
        public long Groesse = -1;
        public long Geladen;
        public string Status = "läuft";
        public string Fehler = "";
        public DateTime Start = DateTime.Now;
        public DateTime Ende;
        internal readonly Dictionary<string, long> anteile = new Dictionary<string, long>();
        internal volatile bool abgebrochen;
        internal readonly List<IDisposable> offen = new List<IDisposable>();

        public Dictionary<string, long> Anteile()
        {
            lock (anteile) return new Dictionary<string, long>(anteile);
        }

        internal void Zaehlen(string weg, long n)
        {
            Interlocked.Add(ref Geladen, n);
            lock (anteile)
            {
                long w;
                anteile.TryGetValue(weg, out w);
                anteile[weg] = w + n;
            }
        }

        public double Mbit
        {
            get
            {
                var e = Ende == default(DateTime) ? DateTime.Now : Ende;
                var d = (e - Start).TotalSeconds;
                return d > 0 ? Interlocked.Read(ref Geladen) * 8 / 1e6 / d : 0;
            }
        }
    }

    sealed class Bremse : Exception
    {
        public readonly int WartenMs;
        public Bremse(int ms) : base("Server bremst") { WartenMs = ms; }
    }

    /// <summary>
    /// Lädt eine Datei in Abschnitten gleichzeitig über alle Wege. Anfangs ein Abschnitt je
    /// Verbindung; wird eine frei, übernimmt sie die hintere Hälfte des größten. So bleibt
    /// die Zahl der Anfragen klein, was Server mit Anfragebegrenzung schont.
    /// </summary>
    public sealed class Lader
    {
        const int JeWeg = 3;
        const long MinTeil = 1 << 20;
        const int Stillstand = 20000;
        const int FehlerProWeg = 5;

        readonly Verteiler v;
        readonly Func<string> ordner;
        readonly List<Auftrag> liste = new List<Auftrag>();
        int naechste;
        public Action<string> Protokoll = _ => { };

        public Lader(Verteiler v, Func<string> ordner)
        {
            this.v = v;
            this.ordner = ordner;
        }

        public List<Auftrag> Liste() { lock (liste) return liste.ToList(); }

        public Auftrag Starten(string adresse)
        {
            Uri u;
            if (!Uri.TryCreate((adresse ?? "").Trim(), UriKind.Absolute, out u) || (u.Scheme != "http" && u.Scheme != "https"))
                throw new ArgumentException("Die Adresse muss mit http:// oder https:// beginnen.");
            var a = new Auftrag { Url = u.ToString() };
            lock (liste)
            {
                a.ID = ++naechste;
                liste.Insert(0, a);
                if (liste.Count > 30) liste.RemoveAt(liste.Count - 1);
            }
            new Thread(() =>
            {
                try
                {
                    Laden(a, u);
                    a.Status = "fertig";
                }
                catch (Exception e)
                {
                    a.Status = a.abgebrochen ? "abgebrochen" : "Fehler";
                    a.Fehler = a.abgebrochen ? "" : e.Message;
                    Protokoll("Download " + a.ID + ": " + e.Message);
                }
                a.Ende = DateTime.Now;
            }) { IsBackground = true, Name = "Download " + a.ID }.Start();
            return a;
        }

        public void Abbrechen(Auftrag a)
        {
            a.abgebrochen = true;
            lock (a.offen) foreach (var s in a.offen) try { s.Dispose(); } catch { }
        }

        void Laden(Auftrag a, Uri u)
        {
            var pfade = v.Pfade();
            if (pfade.Count == 0) throw new IOException("kein Weg ins Internet");
            // Größe und Teilabrufe erfragen, Weiterleitungen folgen
            HttpAntwort probe = null;
            for (int i = 0; i < 8; i++)
            {
                probe = HttpAntwort.Holen(v, pfade[0], u, "0-0", Stillstand);
                if (probe.Status >= 300 && probe.Status < 400 && probe["Location"] != null)
                {
                    u = new Uri(u, probe["Location"]);
                    probe.Dispose();
                    continue;
                }
                break;
            }
            long groesse = -1;
            using (probe)
            {
                if (probe.Status >= 400) throw new IOException("Server antwortet " + probe.Status + " " + probe.StatusText);
                var cr = probe["Content-Range"];
                if (probe.Status == 206 && cr != null && cr.Contains("/"))
                    long.TryParse(cr.Substring(cr.LastIndexOf('/') + 1), out groesse);
                a.Datei = FreierName(ordner(), Dateiname(probe, u));
            }
            var teil = a.Datei + ".teil";
            try
            {
                using (var f = new FileStream(teil, FileMode.Create, FileAccess.Write, FileShare.Read, 1 << 16))
                {
                    if (groesse > 4 * MinTeil)
                    {
                        a.Groesse = groesse;
                        f.SetLength(groesse);
                        InAbschnitten(a, f, pfade, u, groesse);
                    }
                    else AmStueck(a, f, pfade[0], u);
                }
                if (a.abgebrochen) throw new OperationCanceledException();
                File.Move(teil, a.Datei);
            }
            catch
            {
                try { File.Delete(teil); } catch { }
                throw;
            }
        }

        void AmStueck(Auftrag a, FileStream f, Pfad p, Uri u)
        {
            using (var r = HttpAntwort.Holen(v, p, u, null, Stillstand))
            {
                lock (a.offen) a.offen.Add(r);
                if (a.abgebrochen) throw new OperationCanceledException();
                if (r.Status != 200) throw new IOException("Server antwortet " + r.Status + " " + r.StatusText);
                long n;
                if (long.TryParse(r["Content-Length"], out n)) a.Groesse = n;
                var b = new byte[256 * 1024];
                int k;
                while ((k = r.Inhalt.Read(b, 0, b.Length)) > 0)
                {
                    f.Write(b, 0, k);
                    a.Zaehlen(p.Name, k);
                }
            }
        }

        sealed class Abschnitt
        {
            public long Pos, Ende;
            public bool Belegt;
        }

        void InAbschnitten(Auftrag a, FileStream f, List<Pfad> pfade, Uri u, long groesse)
        {
            var sperre = new object();
            var teile = new List<Abschnitt>();
            int anfang = (int)Math.Min(pfade.Count * JeWeg, Math.Max(1, groesse / MinTeil));
            for (int i = 0; i < anfang; i++)
                teile.Add(new Abschnitt { Pos = groesse * i / anfang, Ende = groesse * (i + 1) / anfang });
            var schreiben = new object();
            Exception letzter = null;
            var fadenliste = new List<Thread>();

            Func<Abschnitt> nehmen = () =>
            {
                lock (sperre)
                {
                    Abschnitt groesster = null;
                    foreach (var t in teile)
                    {
                        if (t.Pos >= t.Ende) continue;
                        if (!t.Belegt) { t.Belegt = true; return t; }
                        if (groesster == null || t.Ende - t.Pos > groesster.Ende - groesster.Pos) groesster = t;
                    }
                    if (groesster == null || groesster.Ende - groesster.Pos < 2 * MinTeil) return null;
                    long mitte = groesster.Pos + (groesster.Ende - groesster.Pos) / 2;
                    var neu = new Abschnitt { Pos = mitte, Ende = groesster.Ende, Belegt = true };
                    groesster.Ende = mitte;
                    teile.Add(neu);
                    return neu;
                }
            };
            Func<bool> fertig = () => { lock (sperre) return teile.All(t => t.Pos >= t.Ende); };

            foreach (var pfad in pfade)
            {
                var p = pfad;
                int fehlschlaege = 0, aktiv = JeWeg;
                for (int n = 0; n < JeWeg; n++)
                {
                    var th = new Thread(() =>
                    {
                        try
                        {
                            while (!a.abgebrochen && Volatile.Read(ref fehlschlaege) < FehlerProWeg)
                            {
                                var t = nehmen();
                                if (t == null)
                                {
                                    if (fertig()) return;
                                    Thread.Sleep(200);
                                    continue;
                                }
                                try
                                {
                                    Holen(a, p, u, f, schreiben, sperre, t);
                                    Interlocked.Exchange(ref fehlschlaege, 0);
                                }
                                catch (Exception e)
                                {
                                    if (a.abgebrochen) return;
                                    Protokoll("Abschnitt über " + p.Name + ": " + e.Message);
                                    var b = e as Bremse;
                                    int warten = 1000;
                                    if (b != null)
                                    {
                                        warten = b.WartenMs;
                                        if (Volatile.Read(ref aktiv) > 1) return; // Server will weniger gleichzeitig
                                    }
                                    else
                                    {
                                        Interlocked.Increment(ref fehlschlaege);
                                        letzter = new IOException(p.Name + ": " + e.Message, e);
                                    }
                                    Thread.Sleep(warten);
                                }
                                finally
                                {
                                    lock (sperre) t.Belegt = false;
                                }
                            }
                        }
                        finally { Interlocked.Decrement(ref aktiv); }
                    }) { IsBackground = true };
                    fadenliste.Add(th);
                    th.Start();
                }
            }
            foreach (var th in fadenliste) th.Join();
            if (a.abgebrochen) throw new OperationCanceledException();
            if (!fertig()) throw letzter ?? new IOException("Download unvollständig");
        }

        void Holen(Auftrag a, Pfad p, Uri u, FileStream f, object schreiben, object sperre, Abschnitt t)
        {
            long von, bis;
            lock (sperre) { von = t.Pos; bis = t.Ende - 1; }
            using (var r = HttpAntwort.Holen(v, p, u, von + "-" + bis, Stillstand))
            {
                lock (a.offen) a.offen.Add(r);
                try
                {
                    if (a.abgebrochen) throw new OperationCanceledException();
                    if (r.Status == 429 || r.Status == 503)
                    {
                        int s;
                        int ms = int.TryParse(r["Retry-After"], out s) && s > 0 && s < 120 ? s * 1000 : 3000;
                        throw new Bremse(ms);
                    }
                    if (r.Status != 206) throw new IOException("Server antwortet " + r.Status + " statt Teilinhalt");
                    var b = new byte[256 * 1024];
                    while (true)
                    {
                        int n = r.Inhalt.Read(b, 0, b.Length);
                        if (n <= 0) throw new EndOfStreamException("Verbindung vorzeitig zu");
                        long pos, ende;
                        lock (sperre) { pos = t.Pos; ende = t.Ende; }
                        if (n > ende - pos) n = (int)Math.Max(0, ende - pos);
                        if (n > 0)
                        {
                            lock (schreiben)
                            {
                                f.Position = pos;
                                f.Write(b, 0, n);
                            }
                            lock (sperre) { t.Pos += n; pos = t.Pos; ende = t.Ende; }
                            a.Zaehlen(p.Name, n);
                        }
                        if (pos >= ende) return; // fertig, oder ein anderer hat den Rest übernommen
                    }
                }
                finally
                {
                    lock (a.offen) a.offen.Remove(r);
                }
            }
        }

        static string Dateiname(HttpAntwort r, Uri u)
        {
            var cd = r["Content-Disposition"];
            if (cd != null)
            {
                foreach (var teil in cd.Split(';'))
                {
                    var t = teil.Trim();
                    if (t.StartsWith("filename*=", StringComparison.OrdinalIgnoreCase))
                    {
                        var w = t.Substring(10).Trim('"');
                        int i = w.IndexOf("''", StringComparison.Ordinal);
                        if (i >= 0) w = w.Substring(i + 2);
                        return Sauber(Uri.UnescapeDataString(w));
                    }
                    if (t.StartsWith("filename=", StringComparison.OrdinalIgnoreCase))
                        return Sauber(t.Substring(9).Trim('"'));
                }
            }
            var n = Uri.UnescapeDataString(u.AbsolutePath.TrimEnd('/'));
            n = n.Substring(n.LastIndexOf('/') + 1);
            return Sauber(n == "" ? u.Host + ".bin" : n);
        }

        static string Sauber(string n)
        {
            n = n.Replace('\\', '/');
            n = n.Substring(n.LastIndexOf('/') + 1);
            var sb = new StringBuilder();
            foreach (var c in n) sb.Append(c < 32 || "<>:\"/\\|?*".IndexOf(c) >= 0 ? '_' : c);
            n = sb.ToString().Trim('.', ' ');
            return n == "" ? "download.bin" : n;
        }

        static string FreierName(string ordner, string name)
        {
            Directory.CreateDirectory(ordner);
            var ext = Path.GetExtension(name);
            var stamm = Path.GetFileNameWithoutExtension(name);
            for (int i = 0; i < 1000; i++)
            {
                var k = i == 0 ? name : stamm + " (" + i + ")" + ext;
                var p = Path.Combine(ordner, k);
                if (!File.Exists(p) && !File.Exists(p + ".teil")) return p;
            }
            throw new IOException("kein freier Dateiname");
        }
    }
}
