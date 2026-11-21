using System;
using System.Diagnostics;
using System.IO;
using System.Linq;
using System.Runtime.InteropServices;
using Microsoft.Win32;

namespace Bondr
{
    /// <summary>Einstellungen unter HKCU\Software\Bondr.</summary>
    public static class Einstellungen
    {
        const string Schluessel = @"Software\Bondr";

        static RegistryKey Oeffnen() { return Registry.CurrentUser.CreateSubKey(Schluessel); }

        public static bool Systemproxy
        {
            get { using (var k = Oeffnen()) return (int)(k.GetValue("Systemproxy", 0)) == 1; }
            set { using (var k = Oeffnen()) k.SetValue("Systemproxy", value ? 1 : 0, RegistryValueKind.DWord); }
        }

        public static string[] Aus
        {
            get { using (var k = Oeffnen()) return (k.GetValue("Aus", new string[0]) as string[]) ?? new string[0]; }
            set { using (var k = Oeffnen()) k.SetValue("Aus", value ?? new string[0], RegistryValueKind.MultiString); }
        }

        public static string Ordner
        {
            get
            {
                using (var k = Oeffnen())
                {
                    var o = k.GetValue("Ordner") as string;
                    if (!string.IsNullOrEmpty(o)) return o;
                }
                return Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.UserProfile), "Downloads");
            }
            set { using (var k = Oeffnen()) k.SetValue("Ordner", value); }
        }
    }

    /// <summary>Windows-Proxy und Autostart.</summary>
    public static class Systemproxy
    {
        const string Internet = @"Software\Microsoft\Windows\CurrentVersion\Internet Settings";
        const string Run = @"Software\Microsoft\Windows\CurrentVersion\Run";
        const string Ausnahmen = "<local>;localhost;127.*;10.*;172.16.*;172.17.*;172.18.*;172.19.*;172.2*;172.30.*;172.31.*;192.168.*;*.local;*.fritz.box";

        [DllImport("wininet.dll", SetLastError = true)]
        static extern bool InternetSetOption(IntPtr h, int option, IntPtr buffer, int len);

        static void Melden()
        {
            InternetSetOption(IntPtr.Zero, 39, IntPtr.Zero, 0); // SETTINGS_CHANGED
            InternetSetOption(IntPtr.Zero, 37, IntPtr.Zero, 0); // REFRESH
        }

        public static void Setzen(string adresse)
        {
            using (var k = Registry.CurrentUser.CreateSubKey(Internet))
            {
                k.SetValue("ProxyServer", adresse);
                k.SetValue("ProxyOverride", Ausnahmen);
                k.SetValue("ProxyEnable", 1, RegistryValueKind.DWord);
            }
            Melden();
        }

        /// <summary>Schaltet den Proxy ab, aber nur, wenn er noch auf Bondr zeigt.</summary>
        public static void Entfernen(string adresse)
        {
            using (var k = Registry.CurrentUser.CreateSubKey(Internet))
            {
                if ((k.GetValue("ProxyServer") as string) != adresse) return;
                k.SetValue("ProxyEnable", 0, RegistryValueKind.DWord);
            }
            Melden();
        }

        public static bool Aktiv(string adresse)
        {
            using (var k = Registry.CurrentUser.OpenSubKey(Internet))
            {
                if (k == null) return false;
                var an = k.GetValue("ProxyEnable");
                return an is int && (int)an == 1 && (k.GetValue("ProxyServer") as string) == adresse;
            }
        }

        /// <summary>
        /// Solange der Windows-Proxy an ist, muss Bondr nach einem Neustart laufen, sonst
        /// hat der Browser kein Internet. Deshalb gehört beides zusammen.
        /// </summary>
        public static void Autostart(bool an)
        {
            using (var k = Registry.CurrentUser.CreateSubKey(Run))
            {
                if (an) k.SetValue("Bondr", "\"" + Process.GetCurrentProcess().MainModule.FileName + "\" --still");
                else if (k.GetValueNames().Contains("Bondr")) k.DeleteValue("Bondr");
            }
        }

        public static void Schalten(string adresse, bool an)
        {
            if (an) Setzen(adresse); else Entfernen(adresse);
            Autostart(an);
            Einstellungen.Systemproxy = an;
        }
    }
}
