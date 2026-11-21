using System;
using System.Linq;
using System.Threading;
using System.Windows.Forms;

[assembly: System.Runtime.Versioning.TargetFramework(".NETFramework,Version=v4.8", FrameworkDisplayName = ".NET Framework 4.8")]
[assembly: System.Reflection.AssemblyTitle("Bondr")]
[assembly: System.Reflection.AssemblyDescription("Alle Verbindungen gleichzeitig nutzen")]
[assembly: System.Reflection.AssemblyProduct("Bondr")]
[assembly: System.Reflection.AssemblyCompany("Dschonas04")]
[assembly: System.Reflection.AssemblyCopyright("MIT")]
[assembly: System.Reflection.AssemblyVersion(Bondr.Programm.Version + ".0")]
[assembly: System.Reflection.AssemblyFileVersion(Bondr.Programm.Version + ".0")]

namespace Bondr
{
    static class Programm
    {
        public const string Version = "3.0.1";

        [STAThread]
        static int Main(string[] args)
        {
            Application.EnableVisualStyles();
            Application.SetCompatibleTextRenderingDefault(false);

            // Prüfmodus: Bondr.exe --bild <datei.png> zeichnet das Fenster in eine Datei
            if (args.Length >= 2 && args[0] == "--bild")
            {
                using (var f = new Fenster(false, null))
                {
                    f.Show();
                    var bis = DateTime.Now.AddSeconds(5);
                    while (DateTime.Now < bis) { Application.DoEvents(); Thread.Sleep(50); }
                    using (var b = new System.Drawing.Bitmap(f.Width, f.Height))
                    {
                        f.DrawToBitmap(b, new System.Drawing.Rectangle(0, 0, f.Width, f.Height));
                        b.Save(args[1]);
                    }
                    f.GetType().GetMethod("Beenden", System.Reflection.BindingFlags.NonPublic | System.Reflection.BindingFlags.Instance).Invoke(f, null);
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
                using (var f = new Fenster(args.Contains("--still"), zeigen))
                    Application.Run(f);
                GC.KeepAlive(m);
            }
            return 0;
        }
    }
}
