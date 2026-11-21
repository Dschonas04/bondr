using System.Diagnostics;
using System.Linq;
using Microsoft.Win32;

namespace Bondr
{
    /// <summary>Autostart der App im Benutzerkonto.</summary>
    public static class Autostart
    {
        const string Run = @"Software\Microsoft\Windows\CurrentVersion\Run";

        public static bool An
        {
            get
            {
                using (var k = Registry.CurrentUser.OpenSubKey(Run))
                    return k != null && k.GetValueNames().Contains("Bondr");
            }
            set
            {
                using (var k = Registry.CurrentUser.CreateSubKey(Run))
                {
                    if (value) k.SetValue("Bondr", "\"" + Process.GetCurrentProcess().MainModule.FileName + "\" --still");
                    else if (k.GetValueNames().Contains("Bondr")) k.DeleteValue("Bondr");
                }
            }
        }
    }
}
