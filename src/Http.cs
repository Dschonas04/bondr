using System;
using System.Collections.Generic;
using System.Globalization;
using System.IO;
using System.Net.Security;
using System.Security.Authentication;
using System.Text;

namespace Bondr
{
    /// <summary>
    /// Kleiner HTTP/1.1-Client, der über einen bestimmten Weg geht. Eigene Umsetzung, weil
    /// die eingebauten Klassen von Windows Verbindungen je Zielserver teilen und sich nicht
    /// pro Anfrage an einen Adapter binden lassen.
    /// </summary>
    public sealed class HttpAntwort : IDisposable
    {
        public int Status;
        public string StatusText;
        public Dictionary<string, string> Kopf = new Dictionary<string, string>(StringComparer.OrdinalIgnoreCase);
        public Stream Inhalt;
        Stream roh;

        public string this[string name]
        {
            get { string w; return Kopf.TryGetValue(name, out w) ? w : null; }
        }

        public static HttpAntwort Holen(Verteiler v, Pfad p, Uri u, string bereich, int zeitMs)
        {
            Stream s = v.VerbindenUeber(p, u.Host, u.Port);
            try
            {
                var g = s as Gezaehlt;
                if (g != null) { g.ReadTimeout = zeitMs; g.WriteTimeout = zeitMs; }
                if (u.Scheme == "https")
                {
                    var ssl = new SslStream(s, false);
                    ssl.AuthenticateAsClient(u.Host, null, SslProtocols.None, true);
                    s = ssl;
                }
                var sb = new StringBuilder();
                sb.Append("GET ").Append(u.PathAndQuery).Append(" HTTP/1.1\r\n");
                sb.Append("Host: ").Append(u.IsDefaultPort ? u.Host : u.Host + ":" + u.Port).Append("\r\n");
                sb.Append("User-Agent: Bondr/2\r\nAccept: */*\r\nAccept-Encoding: identity\r\nConnection: close\r\n");
                if (bereich != null) sb.Append("Range: bytes=").Append(bereich).Append("\r\n");
                sb.Append("\r\n");
                var b = Encoding.ASCII.GetBytes(sb.ToString());
                s.Write(b, 0, b.Length);
                s.Flush();

                var l = new Leser(s);
                var erste = l.Zeile();
                if (erste == null) throw new IOException("Server hat die Verbindung geschlossen");
                var t = erste.Split(new[] { ' ' }, 3);
                var a = new HttpAntwort { roh = s };
                a.Status = int.Parse(t[1], CultureInfo.InvariantCulture);
                a.StatusText = t.Length > 2 ? t[2] : "";
                string z;
                while (!string.IsNullOrEmpty(z = l.Zeile()))
                {
                    int i = z.IndexOf(':');
                    if (i > 0) a.Kopf[z.Substring(0, i).Trim()] = z.Substring(i + 1).Trim();
                }
                Stream koerper = new MitRest(l.Rest(), s);
                long laenge;
                if ((a["Transfer-Encoding"] ?? "").IndexOf("chunked", StringComparison.OrdinalIgnoreCase) >= 0)
                    koerper = new Gestueckelt(koerper);
                else if (long.TryParse(a["Content-Length"], out laenge))
                    koerper = new Begrenzt(koerper, laenge);
                a.Inhalt = koerper;
                return a;
            }
            catch
            {
                s.Dispose();
                throw;
            }
        }

        public void Dispose()
        {
            try { roh.Dispose(); } catch { }
        }
    }

    sealed class MitRest : Stream
    {
        readonly byte[] rest;
        int pos;
        readonly Stream s;
        public MitRest(byte[] rest, Stream s) { this.rest = rest; this.s = s; }

        public override int Read(byte[] buffer, int offset, int count)
        {
            if (pos < rest.Length)
            {
                int n = Math.Min(count, rest.Length - pos);
                Buffer.BlockCopy(rest, pos, buffer, offset, n);
                pos += n;
                return n;
            }
            return s.Read(buffer, offset, count);
        }

        public override bool CanRead { get { return true; } }
        public override bool CanSeek { get { return false; } }
        public override bool CanWrite { get { return false; } }
        public override long Length { get { throw new NotSupportedException(); } }
        public override long Position { get { throw new NotSupportedException(); } set { throw new NotSupportedException(); } }
        public override void Flush() { }
        public override long Seek(long o, SeekOrigin so) { throw new NotSupportedException(); }
        public override void SetLength(long v) { throw new NotSupportedException(); }
        public override void Write(byte[] b, int o, int c) { throw new NotSupportedException(); }
    }

    sealed class Begrenzt : Stream
    {
        readonly Stream s;
        long uebrig;
        public Begrenzt(Stream s, long n) { this.s = s; uebrig = n; }

        public override int Read(byte[] buffer, int offset, int count)
        {
            if (uebrig <= 0) return 0;
            int n = s.Read(buffer, offset, (int)Math.Min(count, uebrig));
            if (n <= 0) throw new EndOfStreamException("Verbindung vorzeitig zu");
            uebrig -= n;
            return n;
        }

        public override bool CanRead { get { return true; } }
        public override bool CanSeek { get { return false; } }
        public override bool CanWrite { get { return false; } }
        public override long Length { get { throw new NotSupportedException(); } }
        public override long Position { get { throw new NotSupportedException(); } set { throw new NotSupportedException(); } }
        public override void Flush() { }
        public override long Seek(long o, SeekOrigin so) { throw new NotSupportedException(); }
        public override void SetLength(long v) { throw new NotSupportedException(); }
        public override void Write(byte[] b, int o, int c) { throw new NotSupportedException(); }
    }

    sealed class Gestueckelt : Stream
    {
        readonly Stream s;
        long imStueck;
        bool ende;
        public Gestueckelt(Stream s) { this.s = s; }

        string Zeile()
        {
            var sb = new StringBuilder();
            int c;
            while ((c = s.ReadByte()) >= 0 && c != '\n') if (c != '\r') sb.Append((char)c);
            if (c < 0) throw new EndOfStreamException();
            return sb.ToString();
        }

        public override int Read(byte[] buffer, int offset, int count)
        {
            if (ende) return 0;
            if (imStueck == 0)
            {
                var z = Zeile().Split(';')[0].Trim();
                imStueck = long.Parse(z, NumberStyles.HexNumber, CultureInfo.InvariantCulture);
                if (imStueck == 0) { ende = true; return 0; }
            }
            int n = s.Read(buffer, offset, (int)Math.Min(count, imStueck));
            if (n <= 0) throw new EndOfStreamException();
            imStueck -= n;
            if (imStueck == 0) Zeile();
            return n;
        }

        public override bool CanRead { get { return true; } }
        public override bool CanSeek { get { return false; } }
        public override bool CanWrite { get { return false; } }
        public override long Length { get { throw new NotSupportedException(); } }
        public override long Position { get { throw new NotSupportedException(); } set { throw new NotSupportedException(); } }
        public override void Flush() { }
        public override long Seek(long o, SeekOrigin so) { throw new NotSupportedException(); }
        public override void SetLength(long v) { throw new NotSupportedException(); }
        public override void Write(byte[] b, int o, int c) { throw new NotSupportedException(); }
    }
}
