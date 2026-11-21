using System;
using System.Collections.Generic;
using System.Diagnostics;
using System.Drawing;
using System.Globalization;
using System.Linq;
using System.Threading;
using System.Windows.Forms;

namespace Bondr
{
    /// <summary>Hauptfenster mit Wegen, Proxy-Schalter und Downloads, dazu ein Symbol im Infobereich.</summary>
    sealed class Fenster : Form
    {
        static readonly CultureInfo De = new CultureInfo("de-DE");

        readonly Verteiler v;
        readonly Proxy proxy;
        readonly Lader lader;
        readonly System.Windows.Forms.Timer takt = new System.Windows.Forms.Timer { Interval = 1000 };
        readonly NotifyIcon tray = new NotifyIcon();
        readonly bool still;
        bool beenden, aktualisiere, hinweisGezeigt;
        int zaehler;

        readonly Label summe = new Label();
        readonly ListView wege = new ListView();
        readonly CheckBox proxySchalter = new CheckBox();
        readonly TextBox adresse = new TextBox();
        readonly Button laden = new Button();
        readonly Label ordnerText = new Label();
        readonly ListView downloads = new ListView();
        readonly ToolStripMenuItem trayProxy = new ToolStripMenuItem("Windows-Proxy");

        public Fenster(bool still, EventWaitHandle zeigen)
        {
            this.still = still;
            v = new Verteiler(Einstellungen.Aus, null);
            v.AusGeaendert += a => Einstellungen.Aus = a;
            lader = new Lader(v, () => Einstellungen.Ordner) { Protokoll = Programm.Protokoll };
            proxy = new Proxy(v, Programm.ProxyPort);
            try { proxy.Starten(); }
            catch (Exception e)
            {
                Programm.Protokoll("Proxy-Port belegt: " + e.Message);
                MessageBox.Show("Der Proxy-Port " + Programm.ProxyPort + " ist belegt. Downloads gehen trotzdem.", "Bondr", MessageBoxButtons.OK, MessageBoxIcon.Warning);
            }
            if (Einstellungen.Systemproxy)
            {
                try { Systemproxy.Schalten(Programm.ProxyAdresse, true); }
                catch (Exception e) { Programm.Protokoll("Systemproxy: " + e.Message); }
            }

            Aufbauen();
            ThreadPool.RegisterWaitForSingleObject(zeigen, (s, t) =>
            {
                try { BeginInvoke(new Action(Zeigen)); } catch { }
            }, null, -1, false);
            takt.Tick += (s, e) => Ticken();
            takt.Start();
            Ticken();
        }

        void Aufbauen()
        {
            Text = "Bondr";
            Font = new Font("Segoe UI", 9f);
            AutoScaleMode = AutoScaleMode.Dpi;
            ClientSize = new Size(860, 640);
            MinimumSize = new Size(640, 520);
            StartPosition = FormStartPosition.CenterScreen;
            try { Icon = Icon.ExtractAssociatedIcon(Application.ExecutablePath); } catch { }
            BackColor = SystemColors.Window;

            var raster = new TableLayoutPanel { Dock = DockStyle.Fill, ColumnCount = 1, Padding = new Padding(14) };
            raster.RowStyles.Add(new RowStyle(SizeType.AutoSize));
            raster.RowStyles.Add(new RowStyle(SizeType.Percent, 38));
            raster.RowStyles.Add(new RowStyle(SizeType.AutoSize));
            raster.RowStyles.Add(new RowStyle(SizeType.Percent, 62));
            raster.RowStyles.Add(new RowStyle(SizeType.AutoSize));
            Controls.Add(raster);

            // Kopf
            var kopf = new TableLayoutPanel { Dock = DockStyle.Fill, ColumnCount = 2, AutoSize = true, Margin = new Padding(0, 0, 0, 8) };
            kopf.ColumnStyles.Add(new ColumnStyle(SizeType.Percent, 100));
            kopf.ColumnStyles.Add(new ColumnStyle(SizeType.AutoSize));
            var titel = new Label { Text = "Bondr", Font = new Font("Segoe UI Semibold", 18f), AutoSize = true, Margin = new Padding(0) };
            var unter = new Label { Text = "Kabel, WLAN und Handy gleichzeitig nutzen", ForeColor = SystemColors.GrayText, AutoSize = true, Margin = new Padding(2, 0, 0, 0) };
            var links = new FlowLayoutPanel { FlowDirection = FlowDirection.TopDown, AutoSize = true, WrapContents = false, Margin = new Padding(0) };
            links.Controls.Add(titel);
            links.Controls.Add(unter);
            summe.Font = new Font("Segoe UI Semibold", 20f);
            summe.AutoSize = true;
            summe.Anchor = AnchorStyles.Right;
            summe.TextAlign = ContentAlignment.MiddleRight;
            kopf.Controls.Add(links, 0, 0);
            kopf.Controls.Add(summe, 1, 0);
            raster.Controls.Add(kopf, 0, 0);

            // Wege
            var gWege = new GroupBox { Text = "Wege (Haken entfernen, um einen Weg nicht zu benutzen)", Dock = DockStyle.Fill, Padding = new Padding(8) };
            Liste(wege, true);
            wege.Columns.Add("Weg", 150);
            wege.Columns.Add("Art", 70);
            wege.Columns.Add("Adresse", 110);
            wege.Columns.Add("Empfang", 100, HorizontalAlignment.Right);
            wege.Columns.Add("Senden", 90, HorizontalAlignment.Right);
            wege.Columns.Add("Verbindungen", 95, HorizontalAlignment.Right);
            wege.Columns.Add("Empfangen", 90, HorizontalAlignment.Right);
            wege.Columns.Add("Zustand", 120);
            wege.ItemChecked += (s, e) =>
            {
                if (aktualisiere) return;
                v.Schalten((string)e.Item.Tag, e.Item.Checked);
            };
            gWege.Controls.Add(wege);
            raster.Controls.Add(gWege, 0, 1);

            // Proxy
            var gProxy = new GroupBox { Text = "Programme verteilen", Dock = DockStyle.Fill, AutoSize = true, Padding = new Padding(10, 6, 10, 8) };
            var fp = new FlowLayoutPanel { Dock = DockStyle.Fill, FlowDirection = FlowDirection.TopDown, AutoSize = true, WrapContents = false };
            proxySchalter.Text = "Windows-Proxy verwenden";
            proxySchalter.AutoSize = true;
            proxySchalter.Font = new Font(Font, FontStyle.Bold);
            proxySchalter.Click += (s, e) => ProxySchalten(proxySchalter.Checked);
            fp.Controls.Add(proxySchalter);
            fp.Controls.Add(new Label
            {
                AutoSize = true,
                MaximumSize = new Size(800, 0),
                ForeColor = SystemColors.GrayText,
                Text = "Browser und Programme, die den Windows-Proxy beachten, verteilen ihre Verbindungen auf alle Wege. " +
                       "Eine einzelne Verbindung wird nicht schneller, viele gleichzeitige schon. Adressen im LAN gehen direkt. " +
                       "Andere Programme können den Proxy (SOCKS5 oder HTTP) selbst eintragen: " + Programm.ProxyAdresse
            });
            gProxy.Controls.Add(fp);
            raster.Controls.Add(gProxy, 0, 2);

            // Downloads
            var gDl = new GroupBox { Text = "Datei laden (Stücke kommen gleichzeitig über alle Wege)", Dock = DockStyle.Fill, Padding = new Padding(8) };
            var dlRaster = new TableLayoutPanel { Dock = DockStyle.Fill, ColumnCount = 2 };
            dlRaster.ColumnStyles.Add(new ColumnStyle(SizeType.Percent, 100));
            dlRaster.ColumnStyles.Add(new ColumnStyle(SizeType.AutoSize));
            dlRaster.RowStyles.Add(new RowStyle(SizeType.AutoSize));
            dlRaster.RowStyles.Add(new RowStyle(SizeType.AutoSize));
            dlRaster.RowStyles.Add(new RowStyle(SizeType.Percent, 100));
            adresse.Dock = DockStyle.Fill;
            adresse.Margin = new Padding(0, 2, 6, 2);
            SetCue(adresse, "Download-Adresse, z. B. https://…");
            adresse.KeyDown += (s, e) => { if (e.KeyCode == Keys.Enter) { e.SuppressKeyPress = true; Laden(); } };
            laden.Text = "Laden";
            laden.AutoSize = true;
            laden.Click += (s, e) => Laden();
            dlRaster.Controls.Add(adresse, 0, 0);
            dlRaster.Controls.Add(laden, 1, 0);
            var ordnerZeile = new FlowLayoutPanel { AutoSize = true, Dock = DockStyle.Fill, WrapContents = false, Margin = new Padding(0, 2, 0, 4) };
            ordnerText.AutoSize = true;
            ordnerText.ForeColor = SystemColors.GrayText;
            ordnerText.Margin = new Padding(0, 6, 4, 0);
            var ordnerWahl = new LinkLabel { Text = "ändern", AutoSize = true, Margin = new Padding(0, 6, 0, 0) };
            ordnerWahl.LinkClicked += (s, e) => OrdnerWaehlen();
            ordnerZeile.Controls.Add(ordnerText);
            ordnerZeile.Controls.Add(ordnerWahl);
            dlRaster.Controls.Add(ordnerZeile, 0, 1);
            dlRaster.SetColumnSpan(ordnerZeile, 2);
            Liste(downloads, false);
            downloads.Columns.Add("Datei", 220);
            downloads.Columns.Add("Fortschritt", 170);
            downloads.Columns.Add("Mbit/s", 70, HorizontalAlignment.Right);
            downloads.Columns.Add("Anteile", 220);
            downloads.Columns.Add("Status", 120);
            downloads.DoubleClick += (s, e) => ImOrdnerZeigen();
            var menue = new ContextMenuStrip();
            menue.Items.Add("Im Ordner zeigen", null, (s, e) => ImOrdnerZeigen());
            menue.Items.Add("Abbrechen", null, (s, e) => { var a = Gewaehlt(); if (a != null) lader.Abbrechen(a); });
            menue.Items.Add("Adresse kopieren", null, (s, e) => { var a = Gewaehlt(); if (a != null) Clipboard.SetText(a.Url); });
            downloads.ContextMenuStrip = menue;
            dlRaster.Controls.Add(downloads, 0, 2);
            dlRaster.SetColumnSpan(downloads, 2);
            gDl.Controls.Add(dlRaster);
            raster.Controls.Add(gDl, 0, 3);

            // Fuß
            var fuss = new TableLayoutPanel { Dock = DockStyle.Fill, ColumnCount = 2, AutoSize = true, Margin = new Padding(0, 6, 0, 0) };
            fuss.ColumnStyles.Add(new ColumnStyle(SizeType.Percent, 100));
            fuss.ColumnStyles.Add(new ColumnStyle(SizeType.AutoSize));
            fuss.Controls.Add(new Label { Text = "Bondr " + Programm.Version + "  ·  Schließen legt Bondr in den Infobereich", AutoSize = true, ForeColor = SystemColors.GrayText, Anchor = AnchorStyles.Left }, 0, 0);
            var ende = new Button { Text = "Beenden", AutoSize = true };
            ende.Click += (s, e) => Beenden();
            fuss.Controls.Add(ende, 1, 0);
            raster.Controls.Add(fuss, 0, 4);

            // Infobereich
            tray.Icon = Icon ?? SystemIcons.Application;
            tray.Text = "Bondr";
            tray.Visible = true;
            var tm = new ContextMenuStrip();
            var oeffnen = new ToolStripMenuItem("Öffnen", null, (s, e) => Zeigen()) { Font = new Font(Font, FontStyle.Bold) };
            tm.Items.Add(oeffnen);
            trayProxy.Click += (s, e) => ProxySchalten(!trayProxy.Checked);
            tm.Items.Add(trayProxy);
            tm.Items.Add(new ToolStripSeparator());
            tm.Items.Add("Beenden", null, (s, e) => Beenden());
            tray.ContextMenuStrip = tm;
            tray.MouseClick += (s, e) => { if (e.Button == MouseButtons.Left) Zeigen(); };
        }

        static void Liste(ListView l, bool haken)
        {
            l.Dock = DockStyle.Fill;
            l.View = View.Details;
            l.FullRowSelect = true;
            l.CheckBoxes = haken;
            l.HideSelection = false;
            l.MultiSelect = false;
            l.HeaderStyle = ColumnHeaderStyle.Nonclickable;
            typeof(ListView).GetProperty("DoubleBuffered", System.Reflection.BindingFlags.NonPublic | System.Reflection.BindingFlags.Instance).SetValue(l, true, null);
        }

        [System.Runtime.InteropServices.DllImport("user32.dll", CharSet = System.Runtime.InteropServices.CharSet.Unicode)]
        static extern IntPtr SendMessage(IntPtr h, int msg, IntPtr w, string l);

        static void SetCue(TextBox t, string text)
        {
            t.HandleCreated += (s, e) => SendMessage(t.Handle, 0x1501, (IntPtr)1, text); // EM_SETCUEBANNER
        }

        protected override void SetVisibleCore(bool value)
        {
            // Beim Autostart (--still) gleich im Infobereich bleiben
            if (still && !IsHandleCreated)
            {
                CreateHandle();
                value = false;
            }
            base.SetVisibleCore(value);
        }

        void Zeigen()
        {
            Show();
            if (WindowState == FormWindowState.Minimized) WindowState = FormWindowState.Normal;
            Activate();
        }

        protected override void OnFormClosing(FormClosingEventArgs e)
        {
            if (!beenden && e.CloseReason == CloseReason.UserClosing)
            {
                e.Cancel = true;
                Hide();
                if (!hinweisGezeigt)
                {
                    hinweisGezeigt = true;
                    tray.ShowBalloonTip(3000, "Bondr läuft weiter", "Rechtsklick auf das Symbol zum Beenden.", ToolTipIcon.Info);
                }
                return;
            }
            takt.Stop();
            proxy.Stoppen();
            // Proxy freigeben, sonst hätten Browser kein Internet; die Einstellung bleibt
            try { Systemproxy.Entfernen(Programm.ProxyAdresse); } catch { }
            tray.Visible = false;
            base.OnFormClosing(e);
        }

        void Beenden()
        {
            beenden = true;
            Close();
        }

        void ProxySchalten(bool an)
        {
            try { Systemproxy.Schalten(Programm.ProxyAdresse, an); }
            catch (Exception e) { MessageBox.Show(e.Message, "Bondr", MessageBoxButtons.OK, MessageBoxIcon.Error); }
            Ticken();
        }

        void Laden()
        {
            try
            {
                lader.Starten(adresse.Text);
                adresse.Clear();
                Ticken();
            }
            catch (Exception e)
            {
                MessageBox.Show(e.Message, "Bondr", MessageBoxButtons.OK, MessageBoxIcon.Warning);
            }
        }

        void OrdnerWaehlen()
        {
            using (var d = new FolderBrowserDialog { SelectedPath = Einstellungen.Ordner, Description = "Ordner für Downloads" })
                if (d.ShowDialog(this) == DialogResult.OK) Einstellungen.Ordner = d.SelectedPath;
            Ticken();
        }

        Auftrag Gewaehlt()
        {
            return downloads.SelectedItems.Count == 1 ? downloads.SelectedItems[0].Tag as Auftrag : null;
        }

        void ImOrdnerZeigen()
        {
            var a = Gewaehlt();
            if (a != null && a.Status == "fertig") Process.Start("explorer.exe", "/select,\"" + a.Datei + "\"");
        }

        static string Zahl(double x, int stellen) { return x.ToString("N" + stellen, De); }

        static string Groesse(double b)
        {
            if (b >= 1e9) return Zahl(b / 1e9, 2) + " GB";
            if (b >= 1e6) return Zahl(b / 1e6, 1) + " MB";
            return Zahl(b / 1e3, 0) + " KB";
        }

        void Ticken()
        {
            v.Messen();
            if (++zaehler % 3 == 0) v.Aktualisieren();
            var alle = v.Alle();
            double gesamt = alle.Sum(p => p.RateRein) * 8 / 1e6;
            summe.Text = Zahl(gesamt, 1) + " Mbit/s";
            var tip = "Bondr: " + Zahl(gesamt, 0) + " Mbit/s, " + alle.Count + (alle.Count == 1 ? " Weg" : " Wege");
            tray.Text = tip.Length > 63 ? tip.Substring(0, 63) : tip;

            bool sp = Einstellungen.Systemproxy && Systemproxy.Aktiv(Programm.ProxyAdresse);
            proxySchalter.Checked = sp;
            trayProxy.Checked = sp;
            ordnerText.Text = "Ziel: " + Einstellungen.Ordner;
            if (!Visible) return;

            aktualisiere = true;
            wege.BeginUpdate();
            try
            {
                var schluessel = new HashSet<string>(alle.Select(p => p.Weg.Schluessel));
                foreach (ListViewItem it in wege.Items.Cast<ListViewItem>().ToList())
                    if (!schluessel.Contains(it.Name)) wege.Items.Remove(it);
                foreach (var p in alle)
                {
                    var it = wege.Items[p.Weg.Schluessel];
                    if (it == null)
                    {
                        it = new ListViewItem(p.Name) { Name = p.Weg.Schluessel, Tag = p.Name };
                        for (int i = 0; i < 7; i++) it.SubItems.Add("");
                        wege.Items.Add(it);
                    }
                    bool an = v.IstAn(p.Name);
                    if (it.Checked != an) it.Checked = an;
                    it.SubItems[1].Text = p.Weg.Art;
                    it.SubItems[2].Text = p.Weg.IP.ToString();
                    it.SubItems[3].Text = Zahl(p.RateRein * 8 / 1e6, 1) + " Mbit/s";
                    it.SubItems[4].Text = Zahl(p.RateRaus * 8 / 1e6, 1) + " Mbit/s";
                    it.SubItems[5].Text = Interlocked.Read(ref p.Offen).ToString(De);
                    it.SubItems[6].Text = Groesse(Interlocked.Read(ref p.Rein));
                    it.SubItems[7].Text = !an ? "aus" : p.Gestoert ? "gestört: " + p.Fehler : "bereit";
                    it.ForeColor = an ? SystemColors.WindowText : SystemColors.GrayText;
                }
            }
            finally
            {
                wege.EndUpdate();
                aktualisiere = false;
            }

            downloads.BeginUpdate();
            try
            {
                var liste = lader.Liste();
                var ids = new HashSet<string>(liste.Select(a => a.ID.ToString(De)));
                foreach (ListViewItem it in downloads.Items.Cast<ListViewItem>().ToList())
                    if (!ids.Contains(it.Name)) downloads.Items.Remove(it);
                for (int idx = 0; idx < liste.Count; idx++)
                {
                    var a = liste[idx];
                    var key = a.ID.ToString(De);
                    var it = downloads.Items[key];
                    if (it == null)
                    {
                        it = new ListViewItem("") { Name = key, Tag = a };
                        for (int i = 0; i < 4; i++) it.SubItems.Add("");
                        downloads.Items.Insert(Math.Min(idx, downloads.Items.Count), it);
                    }
                    long geladen = Interlocked.Read(ref a.Geladen);
                    it.Text = a.Datei != "" ? System.IO.Path.GetFileName(a.Datei) : a.Url;
                    it.SubItems[1].Text = a.Groesse > 0
                        ? Zahl(100.0 * geladen / a.Groesse, 0) + " %  (" + Groesse(geladen) + " von " + Groesse(a.Groesse) + ")"
                        : Groesse(geladen);
                    it.SubItems[2].Text = Zahl(a.Mbit, 0);
                    var anteile = a.Anteile();
                    it.SubItems[3].Text = string.Join(", ", anteile.OrderByDescending(kv => kv.Value)
                        .Select(kv => kv.Key + " " + Zahl(geladen > 0 ? 100.0 * kv.Value / geladen : 0, 0) + " %"));
                    it.SubItems[4].Text = a.Fehler != "" ? a.Status + ": " + a.Fehler : a.Status;
                    it.ForeColor = a.Status == "Fehler" ? Color.Firebrick : SystemColors.WindowText;
                }
            }
            finally { downloads.EndUpdate(); }
        }

        protected override void Dispose(bool disposing)
        {
            if (disposing)
            {
                takt.Dispose();
                tray.Dispose();
            }
            base.Dispose(disposing);
        }
    }
}
