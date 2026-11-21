using System;
using System.Collections;
using System.Collections.Generic;
using System.Diagnostics;
using System.Globalization;
using System.IO;
using System.Net;
using System.Text;
using System.Web.Script.Serialization;

namespace Bondr
{
    /// <summary>Startet bondrd.exe neben der App und spricht mit ihm über 127.0.0.1.</summary>
    public sealed class Kern
    {
        public const int Port = 47801;
        static readonly JavaScriptSerializer Json = new JavaScriptSerializer();
        Process prozess;
        string schluessel;

        static string SchluesselDatei
        {
            get { return Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.ApplicationData), "Bondr", "api-schluessel"); }
        }

        public void Starten()
        {
            var exe = Path.Combine(AppDomain.CurrentDomain.BaseDirectory, "bondrd.exe");
            if (!File.Exists(exe)) throw new FileNotFoundException("bondrd.exe fehlt neben Bondr.exe", exe);
            var si = new ProcessStartInfo(exe, "--eltern " + Process.GetCurrentProcess().Id)
            {
                UseShellExecute = false,
                CreateNoWindow = true,
                WindowStyle = ProcessWindowStyle.Hidden
            };
            prozess = Process.Start(si);
        }

        public void Beenden()
        {
            try { Senden("/beenden", new Dictionary<string, object>()); } catch { }
            try { if (prozess != null && !prozess.WaitForExit(2000)) prozess.Kill(); } catch { }
        }

        HttpWebRequest Anfrage(string pfad, string methode)
        {
            if (schluessel == null && File.Exists(SchluesselDatei)) schluessel = File.ReadAllText(SchluesselDatei).Trim();
            if (schluessel == null) throw new IOException("Kern noch nicht bereit");
            var r = (HttpWebRequest)WebRequest.Create("http://127.0.0.1:" + Port + pfad);
            r.Method = methode;
            r.Proxy = null; // nie über den Systemproxy, der zeigt ja auf Bondr selbst
            r.Timeout = 3000;
            r.Headers["X-Bondr"] = schluessel;
            return r;
        }

        static string Lesen(WebResponse a)
        {
            using (var s = new StreamReader(a.GetResponseStream(), Encoding.UTF8)) return s.ReadToEnd();
        }

        public Dictionary<string, object> Stand()
        {
            try
            {
                using (var a = Anfrage("/stand", "GET").GetResponse())
                    return Json.Deserialize<Dictionary<string, object>>(Lesen(a));
            }
            catch (WebException e)
            {
                var h = e.Response as HttpWebResponse;
                if (h != null && h.StatusCode == HttpStatusCode.Forbidden) schluessel = null;
                throw;
            }
        }

        /// <summary>Schickt eine Änderung; wirft mit der Meldung des Kerns, wenn er ablehnt.</summary>
        public void Senden(string pfad, Dictionary<string, object> inhalt)
        {
            var r = Anfrage(pfad, "POST");
            r.ContentType = "application/json";
            var b = Encoding.UTF8.GetBytes(Json.Serialize(inhalt));
            using (var s = r.GetRequestStream()) s.Write(b, 0, b.Length);
            try { using (r.GetResponse()) { } }
            catch (WebException e)
            {
                if (e.Response != null) throw new InvalidOperationException(Lesen(e.Response).Trim());
                throw;
            }
        }

        public static Dictionary<string, object> D(params object[] kv)
        {
            var d = new Dictionary<string, object>();
            for (int i = 0; i + 1 < kv.Length; i += 2) d[(string)kv[i]] = kv[i + 1];
            return d;
        }

        // Hilfen zum Lesen der JSON-Antwort
        public static string S(Dictionary<string, object> d, string k)
        {
            object o;
            return d.TryGetValue(k, out o) && o != null ? Convert.ToString(o, CultureInfo.InvariantCulture) : "";
        }

        public static double N(Dictionary<string, object> d, string k)
        {
            object o;
            return d.TryGetValue(k, out o) && o != null ? Convert.ToDouble(o, CultureInfo.InvariantCulture) : 0;
        }

        public static bool B(Dictionary<string, object> d, string k)
        {
            object o;
            return d.TryGetValue(k, out o) && o is bool && (bool)o;
        }

        public static Dictionary<string, object> O(Dictionary<string, object> d, string k)
        {
            object o;
            return d.TryGetValue(k, out o) && o is Dictionary<string, object> ? (Dictionary<string, object>)o : new Dictionary<string, object>();
        }

        public static List<Dictionary<string, object>> L(Dictionary<string, object> d, string k)
        {
            var aus = new List<Dictionary<string, object>>();
            object o;
            if (d.TryGetValue(k, out o) && o is IEnumerable && !(o is string))
                foreach (var x in (IEnumerable)o)
                {
                    var e = x as Dictionary<string, object>;
                    if (e != null) aus.Add(e);
                }
            return aus;
        }
    }
}
