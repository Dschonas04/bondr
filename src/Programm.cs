using System;
using System.IO;
using System.Linq;
using System.Net;
using System.Threading;
using System.Windows.Forms;

[assembly: System.Runtime.Versioning.TargetFramework(".NETFramework,Version=v4.8", FrameworkDisplayName = ".NET Framework 4.8")]
[assembly: System.Reflection.AssemblyTitle("Bondr")]
[assembly: System.Reflection.AssemblyDescription("Alle Internetzugänge gleichzeitig nutzen")]
[assembly: System.Reflection.AssemblyProduct("Bondr")]
[assembly: System.Reflection.AssemblyCompany("Dschonas04")]
[assembly: System.Reflection.AssemblyCopyright("MIT")]
[assembly: System.Reflection.AssemblyVersion(Bondr.Programm.Version + ".0")]
[assembly: System.Reflection.AssemblyFileVersion(Bondr.Programm.Version + ".0")]

namespace Bondr
{
    static class Programm
    {
        public const string Version = "2.0.0";
        public const int ProxyPort = 1080;
        public static string ProxyAdresse { get { return "127.0.0.1:" + ProxyPort; } }
        public static readonly string Ordner = Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.ApplicationData), "Bondr");

        static readonly object protokollSperre = new object();

        public static void Protokoll(string text)
        {
            try
            {
                lock (protokollSperre)
                {
                    Directory.CreateDirectory(Ordner);
                    File.AppendAllText(Path.Combine(Ordner, "bondr.log"), DateTime.Now.ToString("yyyy-MM-dd HH:mm:ss ") + text + Environment.NewLine);
                }
            }
            catch { }
        }

        [STAThread]
        static int Main(string[] args)
        {
            ServicePointManager.SecurityProtocol = SecurityProtocolType.SystemDefault;
            // Prüfmodus für die Kommandozeile: Bondr.exe --messen <URL> [Ordner]
            if (args.Length >= 2 && args[0] == "--messen") return Messen(args);

            // Bildschirmfoto des Fensters für die Prüfung: Bondr.exe --bild <datei.png>
            if (args.Length >= 2 && args[0] == "--bild")
            {
                Application.EnableVisualStyles();
                using (var f = new Fenster(false, new EventWaitHandle(false, EventResetMode.AutoReset)))
                {
                    f.Show();
                    var bis = DateTime.Now.AddSeconds(4);
                    while (DateTime.Now < bis) { Application.DoEvents(); Thread.Sleep(50); }
                    using (var b = new System.Drawing.Bitmap(f.Width, f.Height))
                    {
                        f.DrawToBitmap(b, new System.Drawing.Rectangle(0, 0, f.Width, f.Height));
                        b.Save(args[1]);
                    }
                }
                return 0;
            }

            bool erste;
            using (var m = new Mutex(true, @"Local\Bondr-Einzeln", out erste))
            using (var zeigen = new EventWaitHandle(false, EventResetMode.AutoReset, @"Local\Bondr-Zeigen"))
            {
                if (!erste)
                {
                    zeigen.Set(); // laufendes Fenster nach vorn holen
                    return 0;
                }
                try { Directory.CreateDirectory(Ordner); File.WriteAllText(Path.Combine(Ordner, "bondr.log"), ""); } catch { }
                Application.EnableVisualStyles();
                Application.SetCompatibleTextRenderingDefault(false);
                Application.ThreadException += (s, e) => Protokoll("Fehler: " + e.Exception);
                var still = args.Contains("--still");
                using (var f = new Fenster(still, zeigen))
                    Application.Run(f);
                GC.KeepAlive(m);
            }
            return 0;
        }

        static int Messen(string[] args)
        {
            var v = new Verteiler(Einstellungen.Aus, null);
            var ordner = args.Length > 2 ? args[2] : Path.GetTempPath();
            var l = new Lader(v, () => ordner);
            using (var aus = new StreamWriter(Path.Combine(ordner, "messung.txt")) { AutoFlush = true })
            {
                aus.WriteLine("Wege: " + string.Join(", ", v.Pfade().Select(p => p.Name + " (" + p.Weg.Art + ", " + p.Weg.IP + ")")));
                l.Protokoll = t => aus.WriteLine("  " + t);
                var a = l.Starten(args[1]);
                double spitze = 0;
                long vorher = 0;
                while (a.Status == "läuft")
                {
                    Thread.Sleep(1000);
                    long jetzt = Interlocked.Read(ref a.Geladen);
                    spitze = Math.Max(spitze, (jetzt - vorher) * 8 / 1e6);
                    vorher = jetzt;
                }
                aus.WriteLine("{0}: {1} MB in {2:F0} s, Schnitt {3:F0} Mbit/s, Spitze {4:F0} Mbit/s {5}",
                    a.Status, a.Geladen / 1048576, (a.Ende - a.Start).TotalSeconds, a.Mbit, spitze, a.Fehler);
                foreach (var kv in a.Anteile()) aus.WriteLine("  {0}: {1} MB", kv.Key, kv.Value / 1048576);
                if (a.Status == "fertig") try { File.Delete(a.Datei); } catch { }
                return a.Status == "fertig" ? 0 : 1;
            }
        }
    }
}
