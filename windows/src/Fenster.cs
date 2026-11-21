using System;
using System.Collections.Generic;
using System.Diagnostics;
using System.Drawing;
using System.Globalization;
using System.IO;
using System.Linq;
using System.Threading;
using System.Windows.Forms;

namespace Bondr
{
    /// <summary>Hauptfenster: Wege mit Rolle und Gewicht, Strategie, Proxy, Downloads; dazu das Symbol im Infobereich.</summary>
    sealed class Fenster : Form
    {
        static readonly CultureInfo De = new CultureInfo("de-DE");
        static readonly string[] Rollen = { "haupt", "reserve", "aus" };
        static readonly string[] RollenText = { "Haupt", "Reserve", "Aus" };

        readonly Kern kern = new Kern();
        readonly System.Windows.Forms.Timer takt = new System.Windows.Forms.Timer { Interval = 1000 };
        readonly NotifyIcon tray = new NotifyIcon();
        readonly bool still;
        bool beenden, aktualisiere, hinweisGezeigt;
        int holt;
        Dictionary<string, object> stand;

        readonly Label summe = new Label();
        readonly Label meldung = new Label();
        readonly ListView wege = new ListView();
        readonly Label auswahl = new Label();
        readonly ComboBox rolle = new ComboBox();
        readonly NumericUpDown gewicht = new NumericUpDown();
        readonly RadioButton sWenigste = new RadioButton(), sReihum = new RadioButton(), sAusfall = new RadioButton();
        readonly Label erklaerung = new Label();
        readonly CheckBox proxySchalter = new CheckBox();
        readonly CheckBox anmeldung = new CheckBox();
        readonly TextBox adresse = new TextBox();
        readonly Label ordnerText = new Label();
        readonly ListView downloads = new ListView();
        readonly ToolStripMenuItem trayProxy = new ToolStripMenuItem("Windows-Proxy");

        public Fenster(bool still, EventWaitHandle zeigen)
        {
            this.still = still;
            Aufbauen();
            try { kern.Starten(); }
            catch (Exception e) { meldung.Text = e.Message; }
            if (zeigen != null)
                ThreadPool.RegisterWaitForSingleObject(zeigen, (s, t) =>
                {
                    try { BeginInvoke(new Action(Zeigen)); } catch { }
                }, null, -1, false);
            takt.Tick += (s, e) => Holen();
            takt.Start();
        }

        void Aufbauen()
        {
            Text = "Bondr";
            Font = new Font("Segoe UI", 9f);
            AutoScaleMode = AutoScaleMode.Dpi;
            ClientSize = new Size(900, 720);
            MinimumSize = new Size(700, 600);
            StartPosition = FormStartPosition.CenterScreen;
            try { Icon = Icon.ExtractAssociatedIcon(Application.ExecutablePath); } catch { }
            BackColor = SystemColors.Window;

            var raster = new TableLayoutPanel { Dock = DockStyle.Fill, ColumnCount = 1, Padding = new Padding(14) };
            raster.RowStyles.Add(new RowStyle(SizeType.AutoSize));
            raster.RowStyles.Add(new RowStyle(SizeType.Percent, 45));
            raster.RowStyles.Add(new RowStyle(SizeType.AutoSize));
            raster.RowStyles.Add(new RowStyle(SizeType.Percent, 55));
            raster.RowStyles.Add(new RowStyle(SizeType.AutoSize));
            Controls.Add(raster);

            // Kopf
            var kopf = new TableLayoutPanel { Dock = DockStyle.Fill, ColumnCount = 2, AutoSize = true, Margin = new Padding(0, 0, 0, 6) };
            kopf.ColumnStyles.Add(new ColumnStyle(SizeType.Percent, 100));
            kopf.ColumnStyles.Add(new ColumnStyle(SizeType.AutoSize));
            var links = new FlowLayoutPanel { FlowDirection = FlowDirection.TopDown, AutoSize = true, WrapContents = false, Margin = new Padding(0) };
            links.Controls.Add(new Label { Text = "Bondr", Font = new Font("Segoe UI Semibold", 18f), AutoSize = true, Margin = new Padding(0) });
            links.Controls.Add(new Label { Text = "Alle Verbindungen gleichzeitig nutzen, wie ein Lastverteiler", ForeColor = SystemColors.GrayText, AutoSize = true, Margin = new Padding(2, 0, 0, 0) });
            meldung.AutoSize = true;
            meldung.ForeColor = Color.DarkOrange;
            meldung.Margin = new Padding(2, 4, 0, 0);
            links.Controls.Add(meldung);
            summe.Font = new Font("Segoe UI Semibold", 20f);
            summe.AutoSize = true;
            summe.Anchor = AnchorStyles.Right;
            summe.Text = "0,0 Mbit/s";
            kopf.Controls.Add(links, 0, 0);
            kopf.Controls.Add(summe, 1, 0);
            raster.Controls.Add(kopf, 0, 0);

            // Wege
            var gWege = new GroupBox { Text = "Wege", Dock = DockStyle.Fill, Padding = new Padding(8) };
            var wRaster = new TableLayoutPanel { Dock = DockStyle.Fill, ColumnCount = 1 };
            wRaster.RowStyles.Add(new RowStyle(SizeType.Percent, 100));
            wRaster.RowStyles.Add(new RowStyle(SizeType.AutoSize));
            Liste(wege);
            wege.Columns.Add("Weg", 150);
            wege.Columns.Add("Art", 65);
            wege.Columns.Add("Adresse", 105);
            wege.Columns.Add("Empfang", 95, HorizontalAlignment.Right);
            wege.Columns.Add("Senden", 85, HorizontalAlignment.Right);
            wege.Columns.Add("Offen", 50, HorizontalAlignment.Right);
            wege.Columns.Add("Rolle", 65);
            wege.Columns.Add("Gewicht", 60, HorizontalAlignment.Right);
            wege.Columns.Add("Zustand", 140);
            wege.SelectedIndexChanged += (s, e) => AuswahlZeigen();
            wRaster.Controls.Add(wege, 0, 0);
            var editor = new FlowLayoutPanel { AutoSize = true, Dock = DockStyle.Fill, WrapContents = false, Margin = new Padding(0, 6, 0, 0) };
            auswahl.AutoSize = true;
            auswahl.Margin = new Padding(0, 6, 12, 0);
            auswahl.Text = "Weg auswählen, um Rolle und Gewicht zu ändern.";
            rolle.DropDownStyle = ComboBoxStyle.DropDownList;
            rolle.Items.AddRange(RollenText);
            rolle.Width = 100;
            rolle.Enabled = false;
            rolle.SelectedIndexChanged += (s, e) => RegelSenden();
            gewicht.Minimum = 1;
            gewicht.Maximum = 10;
            gewicht.Width = 55;
            gewicht.Enabled = false;
            gewicht.ValueChanged += (s, e) => RegelSenden();
            editor.Controls.Add(auswahl);
            editor.Controls.Add(new Label { Text = "Rolle", AutoSize = true, Margin = new Padding(0, 6, 4, 0) });
            editor.Controls.Add(rolle);
            editor.Controls.Add(new Label { Text = "Gewicht", AutoSize = true, Margin = new Padding(12, 6, 4, 0) });
            editor.Controls.Add(gewicht);
            editor.Controls.Add(new Label
            {
                Text = "Reserve springt nur ein, wenn kein Hauptweg geht",
                AutoSize = true, ForeColor = SystemColors.GrayText, Margin = new Padding(14, 6, 0, 0)
            });
            wRaster.Controls.Add(editor, 0, 1);
            gWege.Controls.Add(wRaster);
            raster.Controls.Add(gWege, 0, 1);

            // Verteilung
            var gVert = new GroupBox { Text = "Verteilung", Dock = DockStyle.Fill, AutoSize = true, Padding = new Padding(10, 6, 10, 8) };
            var fv = new FlowLayoutPanel { Dock = DockStyle.Fill, FlowDirection = FlowDirection.TopDown, AutoSize = true, WrapContents = false };
            var strat = new FlowLayoutPanel { AutoSize = true, WrapContents = false, Margin = new Padding(0) };
            sWenigste.Text = "Wenigste Verbindungen";
            sReihum.Text = "Gewichtet reihum";
            sAusfall.Text = "Nur bei Ausfall";
            foreach (var r in new[] { sWenigste, sReihum, sAusfall })
            {
                r.AutoSize = true;
                r.Margin = new Padding(0, 0, 18, 0);
                var rr = r;
                rr.Click += (s, e) => Senden("/strategie", Kern.D("strategie", rr == sReihum ? "reihum" : rr == sAusfall ? "ausfall" : "wenigste"));
                strat.Controls.Add(rr);
            }
            fv.Controls.Add(strat);
            erklaerung.AutoSize = true;
            erklaerung.MaximumSize = new Size(840, 0);
            erklaerung.ForeColor = SystemColors.GrayText;
            erklaerung.Margin = new Padding(2, 2, 0, 8);
            fv.Controls.Add(erklaerung);
            proxySchalter.Text = "Windows-Proxy verwenden (Browser und Programme verteilen ihre Verbindungen nach diesen Regeln, LAN geht direkt)";
            proxySchalter.AutoSize = true;
            proxySchalter.Click += (s, e) => ProxySchalten(proxySchalter.Checked);
            fv.Controls.Add(proxySchalter);
            anmeldung.Text = "Bei Anmeldung starten";
            anmeldung.AutoSize = true;
            anmeldung.Checked = Autostart.An;
            anmeldung.Click += (s, e) => { try { Autostart.An = anmeldung.Checked; } catch (Exception ex) { Fehler(ex); } };
            fv.Controls.Add(anmeldung);
            gVert.Controls.Add(fv);
            raster.Controls.Add(gVert, 0, 2);

            // Downloads
            var gDl = new GroupBox { Text = "Datei laden (Abschnitte kommen gleichzeitig über die Hauptwege)", Dock = DockStyle.Fill, Padding = new Padding(8) };
            var dl = new TableLayoutPanel { Dock = DockStyle.Fill, ColumnCount = 2 };
            dl.ColumnStyles.Add(new ColumnStyle(SizeType.Percent, 100));
            dl.ColumnStyles.Add(new ColumnStyle(SizeType.AutoSize));
            dl.RowStyles.Add(new RowStyle(SizeType.AutoSize));
            dl.RowStyles.Add(new RowStyle(SizeType.AutoSize));
            dl.RowStyles.Add(new RowStyle(SizeType.Percent, 100));
            adresse.Dock = DockStyle.Fill;
            adresse.Margin = new Padding(0, 2, 6, 2);
            Hinweistext(adresse, "Download-Adresse, z. B. https://…");
            adresse.KeyDown += (s, e) => { if (e.KeyCode == Keys.Enter) { e.SuppressKeyPress = true; Laden(); } };
            var laden = new Button { Text = "Laden", AutoSize = true };
            laden.Click += (s, e) => Laden();
            dl.Controls.Add(adresse, 0, 0);
            dl.Controls.Add(laden, 1, 0);
            var oz = new FlowLayoutPanel { AutoSize = true, Dock = DockStyle.Fill, WrapContents = false, Margin = new Padding(0, 2, 0, 4) };
            ordnerText.AutoSize = true;
            ordnerText.ForeColor = SystemColors.GrayText;
            ordnerText.Margin = new Padding(0, 6, 4, 0);
            var ow = new LinkLabel { Text = "ändern", AutoSize = true, Margin = new Padding(0, 6, 0, 0) };
            ow.LinkClicked += (s, e) => OrdnerWaehlen();
            oz.Controls.Add(ordnerText);
            oz.Controls.Add(ow);
            dl.Controls.Add(oz, 0, 1);
            dl.SetColumnSpan(oz, 2);
            Liste(downloads);
            downloads.Columns.Add("Datei", 230);
            downloads.Columns.Add("Fortschritt", 180);
            downloads.Columns.Add("Mbit/s", 65, HorizontalAlignment.Right);
            downloads.Columns.Add("Anteile", 220);
            downloads.Columns.Add("Status", 130);
            downloads.DoubleClick += (s, e) => ImOrdnerZeigen();
            var menue = new ContextMenuStrip();
            menue.Items.Add("Im Ordner zeigen", null, (s, e) => ImOrdnerZeigen());
            menue.Items.Add("Abbrechen", null, (s, e) =>
            {
                var d = GewaehlterDownload();
                if (d != null) Senden("/abbrechen", Kern.D("id", (int)Kern.N(d, "id")));
            });
            downloads.ContextMenuStrip = menue;
            dl.Controls.Add(downloads, 0, 2);
            dl.SetColumnSpan(downloads, 2);
            gDl.Controls.Add(dl);
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
            tm.Items.Add(new ToolStripMenuItem("Öffnen", null, (s, e) => Zeigen()) { Font = new Font(Font, FontStyle.Bold) });
            trayProxy.Click += (s, e) => ProxySchalten(!trayProxy.Checked);
            tm.Items.Add(trayProxy);
            tm.Items.Add(new ToolStripSeparator());
            tm.Items.Add("Beenden", null, (s, e) => Beenden());
            tray.ContextMenuStrip = tm;
            tray.MouseClick += (s, e) => { if (e.Button == MouseButtons.Left) Zeigen(); };
        }

        static void Liste(ListView l)
        {
            l.Dock = DockStyle.Fill;
            l.View = View.Details;
            l.FullRowSelect = true;
            l.HideSelection = false;
            l.MultiSelect = false;
            l.HeaderStyle = ColumnHeaderStyle.Nonclickable;
            typeof(ListView).GetProperty("DoubleBuffered", System.Reflection.BindingFlags.NonPublic | System.Reflection.BindingFlags.Instance).SetValue(l, true, null);
        }

        [System.Runtime.InteropServices.DllImport("user32.dll", CharSet = System.Runtime.InteropServices.CharSet.Unicode)]
        static extern IntPtr SendMessage(IntPtr h, int msg, IntPtr w, string l);

        static void Hinweistext(TextBox t, string text)
        {
            t.HandleCreated += (s, e) => SendMessage(t.Handle, 0x1501, (IntPtr)1, text); // EM_SETCUEBANNER
        }

        protected override void SetVisibleCore(bool value)
        {
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
            Holen();
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
            kern.Beenden(); // der Kern gibt dabei den Windows-Proxy frei
            tray.Visible = false;
            base.OnFormClosing(e);
        }

        void Beenden()
        {
            beenden = true;
            Close();
        }

        void Fehler(Exception e)
        {
            MessageBox.Show(this, e.Message, "Bondr", MessageBoxButtons.OK, MessageBoxIcon.Warning);
        }

        /// <summary>Schickt eine Änderung im Hintergrund und holt danach den neuen Stand.</summary>
        void Senden(string pfad, Dictionary<string, object> inhalt)
        {
            ThreadPool.QueueUserWorkItem(_ =>
            {
                Exception f = null;
                try { kern.Senden(pfad, inhalt); } catch (Exception e) { f = e; }
                try
                {
                    BeginInvoke(new Action(() =>
                    {
                        if (f != null) Fehler(f);
                        Holen();
                    }));
                }
                catch { }
            });
        }

        void ProxySchalten(bool an)
        {
            // Solange der Proxy an ist, muss Bondr nach einem Neustart laufen, sonst hätte
            // der Browser kein Internet. Deshalb schaltet der Proxy den Autostart mit ein.
            if (an)
            {
                try { Autostart.An = true; } catch { }
                anmeldung.Checked = true;
            }
            Senden("/systemproxy", Kern.D("an", an));
        }

        void RegelSenden()
        {
            if (aktualisiere || wege.SelectedItems.Count != 1 || rolle.SelectedIndex < 0) return;
            var name = (string)wege.SelectedItems[0].Tag;
            Senden("/regel", Kern.D("name", name, "gewicht", (int)gewicht.Value, "rolle", Rollen[rolle.SelectedIndex]));
        }

        void AuswahlZeigen()
        {
            if (stand == null) return;
            aktualisiere = true;
            try
            {
                var w = GewaehlterWeg();
                rolle.Enabled = gewicht.Enabled = w != null;
                if (w == null)
                {
                    auswahl.Text = "Weg auswählen, um Rolle und Gewicht zu ändern.";
                    return;
                }
                auswahl.Text = Kern.S(w, "name") + ":";
                rolle.SelectedIndex = Math.Max(0, Array.IndexOf(Rollen, Kern.S(w, "rolle")));
                gewicht.Value = Math.Max(1, Math.Min(10, (int)Kern.N(w, "gewicht")));
            }
            finally { aktualisiere = false; }
        }

        Dictionary<string, object> GewaehlterWeg()
        {
            if (stand == null || wege.SelectedItems.Count != 1) return null;
            var name = (string)wege.SelectedItems[0].Tag;
            return Kern.L(stand, "wege").FirstOrDefault(w => Kern.S(w, "name") == name);
        }

        Dictionary<string, object> GewaehlterDownload()
        {
            if (stand == null || downloads.SelectedItems.Count != 1) return null;
            var id = downloads.SelectedItems[0].Name;
            return Kern.L(stand, "downloads").FirstOrDefault(d => ((int)Kern.N(d, "id")).ToString(De) == id);
        }

        void Laden()
        {
            var url = adresse.Text.Trim();
            if (url == "") return;
            adresse.Clear();
            Senden("/laden", Kern.D("url", url));
        }

        void OrdnerWaehlen()
        {
            using (var d = new FolderBrowserDialog { Description = "Ordner für Downloads" })
            {
                if (stand != null) d.SelectedPath = Kern.S(stand, "ordner");
                if (d.ShowDialog(this) == DialogResult.OK) Senden("/ordner", Kern.D("ordner", d.SelectedPath));
            }
        }

        void ImOrdnerZeigen()
        {
            var d = GewaehlterDownload();
            if (d == null || Kern.S(d, "status") != "fertig") return;
            // Nur echte lokale Dateien an den Explorer geben
            var datei = Kern.S(d, "datei");
            if (datei.IndexOf('"') >= 0 || datei.StartsWith(@"\\") || !Path.IsPathRooted(datei) || !File.Exists(datei)) return;
            Process.Start("explorer.exe", "/select,\"" + Path.GetFullPath(datei) + "\"");
        }

        static string Zahl(double x, int stellen) { return x.ToString("N" + stellen, De); }

        static string Groesse(double b)
        {
            if (b >= 1e9) return Zahl(b / 1e9, 2) + " GB";
            if (b >= 1e6) return Zahl(b / 1e6, 1) + " MB";
            return Zahl(b / 1e3, 0) + " KB";
        }

        void Holen()
        {
            if (Interlocked.Exchange(ref holt, 1) == 1) return;
            ThreadPool.QueueUserWorkItem(_ =>
            {
                Dictionary<string, object> s = null;
                string fehler = null;
                try { s = kern.Stand(); }
                catch (Exception e) { fehler = e.Message; }
                try
                {
                    BeginInvoke(new Action(() =>
                    {
                        Interlocked.Exchange(ref holt, 0);
                        if (s != null) Anzeigen(s);
                        else meldung.Text = "Warte auf den Kern … " + (fehler ?? "");
                    }));
                }
                catch { Interlocked.Exchange(ref holt, 0); }
            });
        }

        void Anzeigen(Dictionary<string, object> s)
        {
            stand = s;
            meldung.Text = "";
            var alle = Kern.L(s, "wege");
            double gesamt = Kern.N(s, "mbit_rein");
            summe.Text = Zahl(gesamt, 1) + " Mbit/s";
            var tip = "Bondr: " + Zahl(gesamt, 0) + " Mbit/s, " + alle.Count + (alle.Count == 1 ? " Weg" : " Wege");
            tray.Text = tip.Length > 63 ? tip.Substring(0, 63) : tip;
            bool sp = Kern.B(s, "systemproxy");
            // Ist der Proxy gewünscht, muss Bondr nach einem Neustart laufen, sonst hätte
            // der Browser kein Internet: Autostart dann immer an und nicht abwählbar.
            bool gewuenscht = Kern.B(s, "gewuenscht");
            if (gewuenscht && !Autostart.An) { try { Autostart.An = true; } catch { } }
            anmeldung.Checked = Autostart.An;
            anmeldung.Enabled = !gewuenscht;
            proxySchalter.Checked = sp;
            trayProxy.Checked = sp;
            ordnerText.Text = "Ziel: " + Kern.S(s, "ordner");
            var st = Kern.S(s, "strategie");
            sWenigste.Checked = st == "wenigste";
            sReihum.Checked = st == "reihum";
            sAusfall.Checked = st == "ausfall";
            erklaerung.Text = st == "reihum"
                ? "Neue Verbindungen gehen reihum, jeder Weg so oft, wie sein Gewicht sagt (wie nginx ohne Zusatz)."
                : st == "ausfall"
                    ? "Alles läuft über den Hauptweg mit dem höchsten Gewicht. Die anderen springen nur ein, wenn er ausfällt."
                    : "Jede neue Verbindung geht an den Weg mit den wenigsten offenen Verbindungen im Verhältnis zu seinem Gewicht (wie nginx least_conn).";
            if (!Visible) return;

            aktualisiere = true;
            wege.BeginUpdate();
            try
            {
                var schl = new HashSet<string>(alle.Select(w => Kern.S(w, "schluessel")));
                foreach (ListViewItem it in wege.Items.Cast<ListViewItem>().ToList())
                    if (!schl.Contains(it.Name)) wege.Items.Remove(it);
                foreach (var w in alle)
                {
                    var key = Kern.S(w, "schluessel");
                    var it = wege.Items[key];
                    if (it == null)
                    {
                        it = new ListViewItem(Kern.S(w, "name")) { Name = key, Tag = Kern.S(w, "name") };
                        for (int i = 0; i < 8; i++) it.SubItems.Add("");
                        wege.Items.Add(it);
                    }
                    var r = Kern.S(w, "rolle");
                    it.SubItems[1].Text = Kern.S(w, "art");
                    it.SubItems[2].Text = Kern.S(w, "ip");
                    it.SubItems[3].Text = Zahl(Kern.N(w, "mbit_rein"), 1) + " Mbit/s";
                    it.SubItems[4].Text = Zahl(Kern.N(w, "mbit_raus"), 1) + " Mbit/s";
                    it.SubItems[5].Text = ((int)Kern.N(w, "offen")).ToString(De);
                    it.SubItems[6].Text = RollenText[Math.Max(0, Array.IndexOf(Rollen, r))];
                    it.SubItems[7].Text = ((int)Kern.N(w, "gewicht")).ToString(De);
                    it.SubItems[8].Text = r == "aus" ? "aus"
                        : Kern.B(w, "gestoert") ? "gestört: " + Kern.S(w, "fehler")
                        : Kern.B(w, "benutzt") ? "aktiv" : "bereit (Reserve)";
                    it.ForeColor = r == "aus" ? SystemColors.GrayText : Kern.B(w, "gestoert") ? Color.Firebrick : SystemColors.WindowText;
                }
            }
            finally
            {
                wege.EndUpdate();
                aktualisiere = false;
            }
            if (!rolle.Focused && !gewicht.Focused) AuswahlZeigen();

            downloads.BeginUpdate();
            try
            {
                var liste = Kern.L(s, "downloads");
                var ids = new HashSet<string>(liste.Select(d => ((int)Kern.N(d, "id")).ToString(De)));
                foreach (ListViewItem it in downloads.Items.Cast<ListViewItem>().ToList())
                    if (!ids.Contains(it.Name)) downloads.Items.Remove(it);
                for (int i = 0; i < liste.Count; i++)
                {
                    var d = liste[i];
                    var key = ((int)Kern.N(d, "id")).ToString(De);
                    var it = downloads.Items[key];
                    if (it == null)
                    {
                        it = new ListViewItem("") { Name = key };
                        for (int k = 0; k < 4; k++) it.SubItems.Add("");
                        downloads.Items.Insert(Math.Min(i, downloads.Items.Count), it);
                    }
                    double geladen = Kern.N(d, "geladen"), groesse = Kern.N(d, "groesse");
                    var datei = Kern.S(d, "datei");
                    it.Text = datei != "" ? Path.GetFileName(datei) : Kern.S(d, "url");
                    it.SubItems[1].Text = groesse > 0
                        ? Zahl(100 * geladen / groesse, 0) + " %  (" + Groesse(geladen) + " von " + Groesse(groesse) + ")"
                        : Groesse(geladen);
                    it.SubItems[2].Text = Zahl(Kern.N(d, "mbit"), 0);
                    it.SubItems[3].Text = string.Join(", ", Kern.L(d, "wege").OrderByDescending(a => Kern.N(a, "bytes"))
                        .Select(a => Kern.S(a, "name") + " " + Zahl(geladen > 0 ? 100 * Kern.N(a, "bytes") / geladen : 0, 0) + " %"));
                    var status = Kern.S(d, "status");
                    var f = Kern.S(d, "fehler");
                    it.SubItems[4].Text = f != "" ? status + ": " + f : status;
                    it.ForeColor = status == "Fehler" ? Color.Firebrick : SystemColors.WindowText;
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
