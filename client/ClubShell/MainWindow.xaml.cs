using System;
using System.Collections.Generic;
using System.Diagnostics;
using System.IO;
using System.Linq;
using System.Runtime.InteropServices;
using System.Text;
using System.Threading;
using System.Threading.Tasks;
using System.Windows;
using System.Windows.Controls;
using System.Windows.Input;
using System.Windows.Interop;
using System.Windows.Media;
using System.Windows.Media.Imaging;
using Club.Core;

namespace Club.Shell;

public partial class MainWindow : Window
{
    // --- P/Invoke ---
    [DllImport("user32.dll")] private static extern bool RegisterHotKey(IntPtr hWnd, int id, uint fsModifiers, uint vk);
    [DllImport("user32.dll")] private static extern bool UnregisterHotKey(IntPtr hWnd, int id);
    [DllImport("user32.dll")] private static extern bool ExitWindowsEx(uint uFlags, uint dwReason);
    [DllImport("gdi32.dll")] private static extern bool DeleteObject(IntPtr hObject);
    [DllImport("user32.dll")] private static extern void keybd_event(byte bVk, byte bScan, uint dwFlags, UIntPtr dwExtraInfo);
    [DllImport("user32.dll")] private static extern int GetKeyboardLayoutList(int nBuff, [Out] IntPtr[] lpList);
    [DllImport("user32.dll")] private static extern IntPtr ActivateKeyboardLayout(IntPtr hkl, uint flags);
    [DllImport("user32.dll")] private static extern IntPtr GetKeyboardLayout(uint idThread);
    [DllImport("user32.dll")] private static extern uint GetWindowThreadProcessId(IntPtr hWnd, IntPtr pid);
    [DllImport("user32.dll")] private static extern int GetKeyboardLayoutName(StringBuilder pwszKLID);
    [DllImport("winmm.dll")] private static extern uint waveOutGetVolume(IntPtr hwo, out uint dwVolume);
    [DllImport("winmm.dll")] private static extern uint waveOutSetVolume(IntPtr hwo, uint dwVolume);

    private const uint MOD_CONTROL = 0x0002, MOD_SHIFT = 0x0004, MOD_NOREPEAT = 0x4000;
    private const uint VK_F12 = 0x7B;
    private const int WM_HOTKEY = 0x0312;
    private const int HOTKEY_EXIT = 1;
    private const uint EWX_SHUTDOWN = 0x0001, EWX_REBOOT = 0x0002, EWX_FORCE = 0x0004;
    private const byte VK_VOLUME_MUTE = 0xAD, VK_VOLUME_DOWN = 0xAE, VK_VOLUME_UP = 0xAF;
    private const uint KLF_ACTIVATE = 0x00000001;

    // --- Перехват Alt+Tab ---
    [DllImport("user32.dll", SetLastError = true)]
    private static extern IntPtr SetWindowsHookEx(int idHook, LowLevelKeyboardProc lpfn, IntPtr hMod, uint dwThreadId);
    [DllImport("user32.dll", SetLastError = true)]
    private static extern bool UnhookWindowsHookEx(IntPtr hhk);
    [DllImport("user32.dll")]
    private static extern IntPtr CallNextHookEx(IntPtr hhk, int nCode, IntPtr wParam, IntPtr lParam);
    [DllImport("user32.dll")]
    private static extern short GetAsyncKeyState(int vKey);
    private delegate IntPtr LowLevelKeyboardProc(int nCode, IntPtr wParam, IntPtr lParam);

    // --- Перечисление окон ---
    [DllImport("user32.dll")] private static extern bool EnumWindows(EnumWindowsProc lpEnumFunc, IntPtr lParam);
    [DllImport("user32.dll", CharSet = CharSet.Unicode)] private static extern int GetWindowText(IntPtr hWnd, StringBuilder text, int count);
    [DllImport("user32.dll")] private static extern bool IsWindowVisible(IntPtr hWnd);
    [DllImport("user32.dll")] private static extern uint GetWindowThreadProcessId(IntPtr hWnd, out uint pid);
    [DllImport("user32.dll")] private static extern int GetWindowLong(IntPtr hWnd, int nIndex);
    [DllImport("user32.dll")] private static extern bool SetForegroundWindow(IntPtr hWnd);
    [DllImport("user32.dll")] private static extern bool BringWindowToTop(IntPtr hWnd);
    [DllImport("user32.dll")] private static extern bool SetWindowPos(IntPtr hWnd, IntPtr hWndInsertAfter, int X, int Y, int cx, int cy, uint uFlags);
    [DllImport("kernel32.dll")] private static extern uint GetCurrentThreadId();
    [DllImport("user32.dll")] private static extern bool AttachThreadInput(uint idAttach, uint idAttachTo, bool fAttach);
    [DllImport("user32.dll")] private static extern bool ShowWindow(IntPtr hWnd, int nCmdShow);
    [DllImport("user32.dll")] private static extern bool IsIconic(IntPtr hWnd);
    [DllImport("user32.dll")] private static extern IntPtr SendMessage(IntPtr hWnd, uint msg, IntPtr wParam, IntPtr lParam);
    [DllImport("user32.dll", EntryPoint = "GetClassLongPtr")] private static extern IntPtr GetClassLongPtr(IntPtr hWnd, int nIndex);
    private delegate bool EnumWindowsProc(IntPtr hWnd, IntPtr lParam);

    private const int WH_KEYBOARD_LL = 13;
    private const int VK_TAB = 0x09;
    private const int VK_MENU = 0x12;
    private const int VK_LMENU = 0xA4;
    private const int VK_RMENU = 0xA5;
    private const int VK_LSHIFT = 0xA0;
    private const int VK_RSHIFT = 0xA1;
    private const int WM_KEYDOWN = 0x0100;
    private const int WM_SYSKEYDOWN = 0x0104;
    private const int WM_KEYUP = 0x0101;
    private const int WM_SYSKEYUP = 0x0105;
    private const int GWL_EXSTYLE = -20;
    private const int WS_EX_TOOLWINDOW = 0x00000080;
    private const int SW_RESTORE = 9;
    private const int SW_SHOW = 5;
    private const uint SWP_NOSIZE = 0x0001;
    private const uint SWP_NOMOVE = 0x0002;
    private static readonly IntPtr HWND_TOP = new IntPtr(0);
    private const uint WM_GETICON = 0x007F;
    private const int GCL_HICON = -14;
    private const int GCL_HICONSM = -34;

    private AgentConfig _cfg;
    private ServerClient _client;
    private SteamDetector _steam;
    private CancellationTokenSource _cts;
    private AgentStatus _status = new();
    private bool _hasData;
    private bool _pwMode;
    private bool _lastNotified;
    private HwndSource _hwndSource;
    private System.Windows.Threading.DispatcherTimer _timer;
    private long _bgVersion = -1;
    private bool _complianceBusy;
    private long _nextComplianceAt;
    private bool _hardwareBusy;
    private long _nextHardwareAt;
    private bool _muted;
    private bool _launching;
    private bool _maintenance;

    private IntPtr _hook;
    private LowLevelKeyboardProc _hookProc;
    private bool _switcherVisible;
    private List<WinInfo> _windows;
    private int _selIndex;
    private bool _switcherPrevTopmost;

    public MainWindow()
    {
        InitializeComponent();
    }

    protected override void OnSourceInitialized(EventArgs e)
    {
        base.OnSourceInitialized(e);
        _hwndSource = (HwndSource)PresentationSource.FromVisual(this);
        _hwndSource.AddHook(WndProc);
        var hwnd = new WindowInteropHelper(this).Handle;
        RegisterHotKey(hwnd, HOTKEY_EXIT, MOD_CONTROL | MOD_SHIFT | MOD_NOREPEAT, VK_F12);
        InstallAltTabHook();
    }

    protected override void OnClosed(EventArgs e)
    {
        var hwnd = new WindowInteropHelper(this).Handle;
        UnregisterHotKey(hwnd, HOTKEY_EXIT);
        UninstallAltTabHook();
        _cts?.Cancel();
        base.OnClosed(e);
    }

    protected override void OnClosing(System.ComponentModel.CancelEventArgs e)
    {
        // Alt+F4 и крестик не должны закрывать киоск — выход только через пароль администратора
        e.Cancel = true;
        base.OnClosing(e);
    }

    private IntPtr WndProc(IntPtr hwnd, int msg, IntPtr wParam, IntPtr lParam, ref bool handled)
    {
        if (msg == WM_HOTKEY && wParam.ToInt32() == HOTKEY_EXIT)
        {
            OpenPassword();
            handled = true;
        }
        return IntPtr.Zero;
    }

    private async void Window_Loaded(object sender, RoutedEventArgs e)
    {
        try
        {
            Log.Write("запуск агента...");
            _cfg = AgentConfig.Load(ConfigPath());
            Log.Write("конфиг: " + ConfigPath() +
                      " | сервер=" + (_cfg.ServerUrl ?? "") +
                      " | пк=" + _cfg.PcId +
                      " | локальный пароль=" + (string.IsNullOrEmpty(_cfg.LocalAdminPassword) ? "НЕ задан" : "задан"));
            _client = new ServerClient(_cfg);
            _steam = new SteamDetector(SteamDetector.DefaultSteamRoot());

            LoadGames();
            _cts = new CancellationTokenSource();
            StartTimer();
            UpdateLangLabel();
            UpdateNetStatus();
            UpdateVolumeSlider();
            _ = LoadBackgroundAsync();
            Log.Write("агент запущен, ПК #" + _cfg.PcId + ", сервер " + _cfg.ServerUrl);
            await PollLoopAsync();
        }
        catch (Exception ex)
        {
            Log.Write("ОШИБКА СТАРТА: " + ex);
            ShowStartupError(ex.Message);
        }
    }

    private void StartTimer()
    {
        _timer = new System.Windows.Threading.DispatcherTimer { Interval = TimeSpan.FromSeconds(1) };
        _timer.Tick += (s, e) => UpdateTimer();
        _timer.Start();
    }

    private void UpdateTimer()
    {
        try
        {
            var now = DateTimeOffset.UtcNow.ToUnixTimeSeconds();
            if (_status.SessionOpen)
            {
                // открытый тариф — считаем прошедшее время (не обратный отсчёт)
                var elapsed = now - _status.SessionStartedAt;
                if (elapsed < 0) elapsed = 0;
                SidebarTimer.Text = FormatTime(elapsed);
            }
            else
            {
                var remaining = _status.EndsAt - now;
                if (remaining < 0) remaining = 0;
                SidebarTimer.Text = FormatTime(remaining);
            }
            ClockText.Text = DateTime.Now.ToString("HH:mm");
            UpdateNetStatus();
        }
        catch { }
    }

    // --- Нижняя панель: язык / интернет / громкость ---

    private void UpdateLangLabel()
    {
        try
        {
            LangBtn.Content = LangCode(CurrentLayoutId());
        }
        catch { LangBtn.Content = "RU"; }
    }

    private static string CurrentLayoutId()
    {
        var sb = new StringBuilder(16);
        return GetKeyboardLayoutName(sb) > 0 ? sb.ToString() : "00000409";
    }

    private static string LangCode(string klid)
    {
        if (klid.Length < 8) return klid;
        var low = klid.Substring(4, 4).ToLowerInvariant();
        switch (low)
        {
            case "0419": return "RU";
            case "0409": return "EN";
            case "0422": return "UK";
            case "0407": return "DE";
            case "040c": return "FR";
            case "040a": return "ES";
            case "0410": return "IT";
            case "0428": return "KY";
            case "0423": return "BY";
            default: return low.ToUpperInvariant();
        }
    }

    private void LangBtn_Click(object sender, RoutedEventArgs e)
    {
        try
        {
            var n = GetKeyboardLayoutList(0, null);
            if (n <= 0) return;
            var list = new IntPtr[n];
            GetKeyboardLayoutList(n, list);
            var hwnd = new WindowInteropHelper(this).Handle;
            var cur = GetKeyboardLayout(GetWindowThreadProcessId(hwnd, IntPtr.Zero));
            var idx = -1;
            for (var i = 0; i < list.Length; i++)
                if (list[i] == cur) { idx = i; break; }
            idx = (idx + 1) % list.Length;
            ActivateKeyboardLayout(list[idx], KLF_ACTIVATE);
            UpdateLangLabel();
        }
        catch { }
    }

    private void UpdateNetStatus()
    {
        try
        {
            var ok = System.Net.NetworkInformation.NetworkInterface.GetIsNetworkAvailable();
            NetStatus.Text = ok ? "\uE839" : "\uE774"; // Ethernet / Globe
            NetStatus.Foreground = (System.Windows.Media.Brush)FindResource(ok ? "Cyan" : "Blood");
            NetStatus.ToolTip = ok ? "Интернет: есть" : "Интернет: нет";
        }
        catch { }
    }

    private bool _volUpdating;

    private void UpdateVolumeSlider()
    {
        try
        {
            uint vol;
            if (waveOutGetVolume(IntPtr.Zero, out vol) == 0)
            {
                var left = vol & 0xFFFF;
                _volUpdating = true;
                VolumeSlider.Value = left * 100.0 / 65535.0;
                _volUpdating = false;
            }
        }
        catch { }
    }

    private void VolumeMute_Click(object sender, RoutedEventArgs e)
    {
        keybd_event(VK_VOLUME_MUTE, 0, 0, UIntPtr.Zero);
        _muted = !_muted;
        VolumeMuteBtn.Content = _muted ? "\uE74F" : "\uE767"; // Mute / Volume
    }

    private void VolumeSlider_ValueChanged(object sender, RoutedPropertyChangedEventArgs<double> e)
    {
        if (_volUpdating) return;
        var v = Math.Max(0, Math.Min(65535, (uint)(VolumeSlider.Value * 65535 / 100.0)));
        var vol = (v & 0xFFFF) | (v << 16);
        waveOutSetVolume(IntPtr.Zero, vol);
    }

    private static string FormatTime(long seconds)
    {
        var t = TimeSpan.FromSeconds(seconds);
        return ((int)t.TotalHours).ToString("00") + ":" + t.Minutes.ToString("00");
    }

    // --- Сайдбар: клиент / гость ---

    private void UpdateSidebar()
    {
        var logged = _status.ClientId > 0 && !string.IsNullOrEmpty(_status.ClientName);
        if (logged)
        {
            SidebarTitle.Text = _status.ClientName;
            SidebarTimerLabel.Text = _status.SessionOpen ? "ВРЕМЯ В СЕАНСЕ" : "ОСТАЛОСЬ ВРЕМЕНИ";
            SidebarTimerLabel.Visibility = Visibility.Visible;
            SidebarTimer.Visibility = Visibility.Visible;
            SidebarBalance.Text = "Баланс: " + _status.ClientBalance.ToString("0.##") + " сом";
            SidebarBonus.Text = "Бонусы: " + _status.ClientBonusPoints;
            SidebarBalance.Visibility = Visibility.Visible;
            SidebarBonus.Visibility = Visibility.Visible;
            RegisterBtn.Visibility = Visibility.Collapsed;
            LogoutBtn.Visibility = Visibility.Visible;
        }
        else
        {
            SidebarTitle.Text = "Гость";
            SidebarTimerLabel.Text = "ОСТАЛОСЬ ВРЕМЕНИ";
            SidebarTimerLabel.Visibility = Visibility.Visible;
            SidebarTimer.Visibility = Visibility.Visible;
            SidebarBalance.Visibility = Visibility.Collapsed;
            SidebarBonus.Visibility = Visibility.Collapsed;
            RegisterBtn.Visibility = Visibility.Visible;
            LogoutBtn.Visibility = Visibility.Collapsed;
        }
    }

    private async Task PollOnceAsync()
    {
        try
        {
            var status = await _client.GetStatusAsync();
            await Dispatcher.InvokeAsync(() => ApplyStatus(status));
        }
        catch { }
    }

    private void ShowToast(string text)
    {
        PillText.Text = text;
        Pill.Visibility = Visibility.Visible;
        _ = Task.Run(async () =>
        {
            await Task.Delay(3000);
            await Dispatcher.InvokeAsync(() =>
            {
                if (PillText.Text == text) Pill.Visibility = Visibility.Collapsed;
            });
        });
    }

    private static string FriendlyError(Exception ex)
    {
        var msg = ex.Message ?? "";
        // HttpRequestException: "Response status code does not indicate success: 404 (Not Found)."
        var i = msg.IndexOf("): ");
        if (i >= 0) msg = msg.Substring(i + 3);
        i = msg.IndexOf(" (");
        if (i >= 0) msg = msg.Substring(0, i);
        return msg.Length == 0 ? "Ошибка" : msg;
    }

    // --- Вход (экран блокировки) / регистрация / выход ---

    private async void LockLoginOk_Click(object sender, RoutedEventArgs e) => await LockLoginAsync();

    private void LockPasswordBox_KeyDown(object sender, KeyEventArgs e)
    {
        if (e.Key == Key.Enter) _ = LockLoginAsync();
    }

    private void LockShowPwd_Click(object sender, RoutedEventArgs e)
    {
        LockPasswordBox.PasswordChar = (LockPasswordBox.PasswordChar == '\0') ? '\u25CF' : '\0';
    }

    private void RegShowPwd_Click(object sender, RoutedEventArgs e)
    {
        var ch = (RegPasswordBox.PasswordChar == '\0') ? '\u25CF' : '\0';
        RegPasswordBox.PasswordChar = ch;
        RegPasswordBox2.PasswordChar = ch;
    }

    private void RegPasswordBox2_KeyDown(object sender, KeyEventArgs e)
    {
        if (e.Key == Key.Enter) _ = RegisterAsync();
    }

    private async Task LockLoginAsync()
    {
        var login = (LockLoginBox.Text ?? "").Trim();
        var password = LockPasswordBox.Password ?? "";
        if (login.Length == 0) { LockLoginError.Text = "Введите логин"; return; }
        if (password.Length == 0) { LockLoginError.Text = "Введите пароль"; return; }
        try
        {
            var result = await _client.ClientLoginAsync(login, password);
            if (!result.SessionStarted)
            {
                LockLoginError.Text = string.IsNullOrEmpty(result.Message) ? "Недостаточно баланса для входа" : result.Message;
                return;
            }
            LockLoginError.Text = "";
            ShowToast("Добро пожаловать, " + result.Client.Name + "!");
            await PollOnceAsync();
        }
        catch (Exception ex)
        {
            LockLoginError.Text = FriendlyError(ex);
        }
    }

    private void RegisterBtn_Click(object sender, RoutedEventArgs e)
    {
        RegNameBox.Text = "";
        RegPhoneBox.Text = "";
        RegPasswordBox.Password = "";
        RegPasswordBox2.Password = "";
        RegShowPwd.IsChecked = false;
        RegisterError.Text = "";
        RegisterView.Visibility = Visibility.Visible;
        RegNameBox.Focus();
    }

    private void LockRegister_Click(object sender, RoutedEventArgs e)
    {
        RegisterView.Visibility = Visibility.Visible;
        RegNameBox.Focus();
    }

    private void RegisterCancel_Click(object sender, RoutedEventArgs e)
    {
        RegisterView.Visibility = Visibility.Collapsed;
    }

    private async void RegisterOk_Click(object sender, RoutedEventArgs e) => await RegisterAsync();

    private async Task RegisterAsync()
    {
        var name = (RegNameBox.Text ?? "").Trim();
        var phone = (RegPhoneBox.Text ?? "").Trim();
        var password = RegPasswordBox.Password ?? "";
        var password2 = RegPasswordBox2.Password ?? "";
        if (name.Length == 0) { RegisterError.Text = "Введите имя"; return; }
        if (password.Length == 0) { RegisterError.Text = "Введите пароль"; return; }
        if (password != password2) { RegisterError.Text = "Пароли не совпадают"; return; }
        try
        {
            var client = await _client.ClientRegisterAsync(name, phone, password);
            RegisterError.Text = "";
            RegisterView.Visibility = Visibility.Collapsed;
            ShowToast("Аккаунт создан: " + client.Name);
            await PollOnceAsync();
        }
        catch (Exception ex)
        {
            RegisterError.Text = FriendlyError(ex);
        }
    }

    private async void LogoutBtn_Click(object sender, RoutedEventArgs e)
    {
        try
        {
            var resp = await _client.ClientLogoutAsync();
            if (resp.TryGetValue("charged", out var cEl) && cEl.GetDouble() > 0)
                ShowToast("Списано за время: " + cEl.GetDouble().ToString("0.##") + " сом");
            else if (resp.TryGetValue("refund", out var rEl) && rEl.GetDouble() > 0)
                ShowToast("Возвращено: " + rEl.GetDouble().ToString("0.##") + " сом");
        }
        catch { }
        await PollOnceAsync();
    }

    // --- Кнопки: поесть / попить / вызвать админа ---

    // --- Магазин: категории → товары → покупка/заказ ---

    private List<ProductInfo> _shopProducts = new();
    private string _currentCategory = "";

    private async void ShopBtn_Click(object sender, RoutedEventArgs e) => await OpenShopAsync();

    private async Task OpenShopAsync()
    {
        _shopProducts = await _client.GetProductsAsync();
        _currentCategory = "";
        ShowCategories();
        MenuView.Visibility = Visibility.Visible;
    }

    private void ShowCategories()
    {
        _currentCategory = "";
        var categories = _shopProducts
            .Select(p => (p.Category ?? "").Trim())
            .Where(c => c.Length > 0)
            .Distinct(StringComparer.OrdinalIgnoreCase)
            .OrderBy(c => c)
            .ToList();
        // товары без категории — отдельная «категория»
        var uncat = _shopProducts.Any(p => string.IsNullOrWhiteSpace(p.Category));
        MenuTitle.Text = "Магазин";
        MenuBackBtn.Visibility = Visibility.Collapsed;
        var items = new List<CategoryVm>();
        foreach (var c in categories)
            items.Add(new CategoryVm { Name = c, PriceLabel = "" });
        if (uncat)
            items.Add(new CategoryVm { Name = "Разное", PriceLabel = "" });
        if (items.Count == 0)
        {
            // нет категорий — сразу товары
            ShowProducts("");
            return;
        }
        MenuList.ItemsSource = items;
    }

    private void ShowProducts(string category)
    {
        _currentCategory = category;
        MenuTitle.Text = string.IsNullOrEmpty(category) ? "Все товары" : category;
        MenuBackBtn.Visibility = Visibility.Visible;
        IEnumerable<ProductInfo> list = _shopProducts;
        if (!string.IsNullOrEmpty(category))
            list = list.Where(p => string.Equals((p.Category ?? "").Trim(), category, StringComparison.OrdinalIgnoreCase));
        else
            list = list.Where(p => string.IsNullOrWhiteSpace(p.Category));
        MenuList.ItemsSource = list.Select(p => new ProductVm { Id = p.Id, Name = p.Name, PriceLabel = p.Price.ToString("0.##") + " сом" }).ToList();
    }

    private void MenuBack_Click(object sender, RoutedEventArgs e)
    {
        ShowCategories();
    }

    private void MenuClose_Click(object sender, RoutedEventArgs e)
    {
        MenuView.Visibility = Visibility.Collapsed;
    }

    private async void Product_Click(object sender, RoutedEventArgs e)
    {
        if (sender is not Button b) return;
        // клик по категории — показать товары категории
        if (b.Tag is CategoryVm cat)
        {
            ShowProducts(cat.Name);
            return;
        }
        if (b.Tag is ProductVm vm)
        {
            MenuView.Visibility = Visibility.Collapsed;
            var loggedIn = _status.ClientId > 0;
            try
            {
                if (loggedIn)
                {
                    // клиент — покупка с баланса
                    var resp = await _client.BuyAsync(vm.Id);
                    var balance = resp.TryGetValue("balance", out var balEl) ? balEl.GetDouble().ToString("0.##") : "";
                    ShowToast("Куплено: " + vm.Name + (balance.Length > 0 ? " · баланс " + balance + " сом" : ""));
                }
                else
                {
                    // гость — заказ, оплата через кассу
                    await _client.CallAdminAsync("Заказ: " + vm.Name);
                    ShowToast("Заказ отправлен администратору: " + vm.Name);
                }
                await PollOnceAsync();
            }
            catch (Exception ex)
            {
                ShowToast(FriendlyError(ex));
            }
        }
    }

    private async void CallAdminBtn_Click(object sender, RoutedEventArgs e)
    {
        try
        {
            await _client.CallAdminAsync("Вызов администратора");
            ShowToast("Администратор вызван");
        }
        catch
        {
            ShowToast("Не удалось вызвать администратора");
        }
    }

    private async Task LoadBackgroundAsync()
    {
        try
        {
            var bytes = await _client.GetBackgroundAsync();
            await Dispatcher.InvokeAsync(() =>
            {
                BgImage.Source = (bytes != null && bytes.Length > 0) ? BytesToImage(bytes) : null;
            });
        }
        catch
        {
            BgImage.Source = null;
        }
    }

    private static BitmapImage BytesToImage(byte[] bytes)
    {
        var bmp = new BitmapImage();
        bmp.BeginInit();
        bmp.CacheOption = BitmapCacheOption.OnLoad;
        bmp.StreamSource = new MemoryStream(bytes);
        bmp.EndInit();
        bmp.Freeze();
        return bmp;
    }

    private async Task SendComplianceAsync()
    {
        if (_complianceBusy) return;
        _complianceBusy = true;
        try
        {
            // PowerShell/WMI блокирует поток — собираем в фоне
            var report = await Task.Run(ComplianceChecker.Collect);
            await _client.PostComplianceAsync(report);
            _nextComplianceAt = DateTimeOffset.UtcNow.ToUnixTimeSeconds() + 3600;
        }
        catch (Exception ex)
        {
            Log.Write("compliance: " + ex.Message);
            _nextComplianceAt = DateTimeOffset.UtcNow.ToUnixTimeSeconds() + 300; // повтор через 5 мин
        }
        finally
        {
            _complianceBusy = false;
        }
    }

    private async Task SendHardwareAsync()
    {
        if (_hardwareBusy) return;
        _hardwareBusy = true;
        try
        {
            var info = await Task.Run(HardwareReporter.Collect);
            await _client.PostHardwareAsync(info);
            _nextHardwareAt = DateTimeOffset.UtcNow.ToUnixTimeSeconds() + 3600;
        }
        catch (Exception ex)
        {
            Log.Write("hardware: " + ex.Message);
            _nextHardwareAt = DateTimeOffset.UtcNow.ToUnixTimeSeconds() + 600; // повтор через 10 мин
        }
        finally
        {
            _hardwareBusy = false;
        }
    }

    private void ShowStartupError(string message)
    {
        ShowShell(topmost: true);
        LockView.Visibility = Visibility.Visible;
        LauncherView.Visibility = Visibility.Collapsed;
        PasswordView.Visibility = Visibility.Collapsed;
        LockTitle.Text = "ОШИБКА ЗАПУСКА";
        LockCode.Text = message;
        LockHint.Text = "Подробности в agent.log";
    }

    /// <summary>Каталог exe — надёжнее AppContext.BaseDirectory, когда агент запущен как оболочка.</summary>
    private static string BaseDir()
    {
        var p = Environment.ProcessPath;
        if (!string.IsNullOrEmpty(p))
        {
            var d = Path.GetDirectoryName(p);
            if (!string.IsNullOrEmpty(d)) return d;
        }
        return AppContext.BaseDirectory;
    }

    private static string ConfigPath()
    {
        return Path.Combine(BaseDir(), "agent.json");
    }

    private void LoadGames()
    {
        try
        {
            var list = new List<GameConfig>();

            // 1) Steam
            var steamGames = _steam.Detect();
            list.AddRange(steamGames);

            // 2) ярлыки .lnk из папки games
            var folder = ResolveGamesFolder();
            var lnkCount = 0;
            if (Directory.Exists(folder))
            {
                foreach (var lnk in Directory.GetFiles(folder, "*.lnk"))
                {
                    list.Add(new GameConfig { Name = Path.GetFileNameWithoutExtension(lnk), Command = lnk });
                    lnkCount++;
                }
            }

            // 3) json
            var jsonCount = _cfg.Games?.Count ?? 0;
            if (_cfg.Games != null)
                list.AddRange(_cfg.Games);

            // 4) фильтр нежелательных игр (exclude_games из agent.json)
            if (_cfg.ExcludeGames != null && _cfg.ExcludeGames.Count > 0)
            {
                var ex = new HashSet<string>(_cfg.ExcludeGames, StringComparer.OrdinalIgnoreCase);
                list.RemoveAll(g => ex.Contains(g.Name));
            }

            var seen = new HashSet<string>(StringComparer.OrdinalIgnoreCase);
            var tiles = new List<GameTileViewModel>();
            foreach (var g in list)
            {
                if (string.IsNullOrWhiteSpace(g.Command) || !seen.Add(g.Command)) continue;
                tiles.Add(new GameTileViewModel { Game = g, Icon = IconFor(g) });
            }
            GamesList.ItemsSource = tiles;
            _ = LoadCoversAsync(tiles);

            Log.Write($"игры: Steam={steamGames.Count}, папка={folder} (есть={Directory.Exists(folder)}, lnk={lnkCount}), json={jsonCount}, плиток={tiles.Count}");
        }
        catch (Exception ex)
        {
            Log.Write("Ошибка загрузки игр: " + ex);
        }
    }

    /// <summary>
    /// Путь к папке с ярлыками. Относительный путь резолвим от каталога exe,
    /// а не от рабочего каталога (при запуске как оболочка он System32).
    /// </summary>
    private string ResolveGamesFolder()
    {
        var f = _cfg.GamesFolder;
        if (string.IsNullOrEmpty(f))
            return Path.Combine(BaseDir(), "games");
        if (!Path.IsPathRooted(f))
            return Path.Combine(BaseDir(), f);
        return f;
    }

    private async Task LoadCoversAsync(List<GameTileViewModel> tiles)
    {
        foreach (var t in tiles)
        {
            try
            {
                if (t.Game.AppId > 0)
                {
                    var cover = await CoverCache.GetAsync(t.Game.AppId);
                    if (cover != null) t.Cover = cover;
                }
                else
                {
                    // кастомная обложка с сервера — по имени ярлыка (нижний регистр)
                    var name = (t.Game.Name ?? "").Trim().ToLowerInvariant();
                    if (name.Length > 0)
                    {
                        var bytes = await _client.GetCustomCoverAsync(name);
                        if (bytes != null && bytes.Length > 0)
                            t.Cover = BytesToImage(bytes);
                    }
                }
            }
            catch (Exception ex)
            {
                Log.Write("Ошибка обложки '" + t.Game.Name + "': " + ex);
            }
        }
    }

    private static ImageSource IconFor(GameConfig g)
    {
        try
        {
            if (g.Command.StartsWith("steam://", StringComparison.OrdinalIgnoreCase)) return null;
            using var icon = System.Drawing.Icon.ExtractAssociatedIcon(g.Command);
            if (icon == null) return null;
            using var bmp = icon.ToBitmap();
            var hbmp = bmp.GetHbitmap();
            try
            {
                return Imaging.CreateBitmapSourceFromHBitmap(hbmp, IntPtr.Zero, Int32Rect.Empty, BitmapSizeOptions.FromEmptyOptions());
            }
            finally { DeleteObject(hbmp); }
        }
        catch { return null; }
    }

    private async Task PollLoopAsync()
    {
        while (!_cts.IsCancellationRequested)
        {
            WriteHeartbeat();

            // FACEIT-отчёт: при старте и раз в час
            if (DateTimeOffset.UtcNow.ToUnixTimeSeconds() >= _nextComplianceAt)
                _ = SendComplianceAsync();

            // Железо и диски: при старте и раз в час
            if (DateTimeOffset.UtcNow.ToUnixTimeSeconds() >= _nextHardwareAt)
                _ = SendHardwareAsync();

            AgentStatus status = null;
            try { status = await _client.GetStatusAsync(); }
            catch { /* связь потеряна */ }

            if (status != null)
                await Dispatcher.InvokeAsync(() => ApplyStatus(status));
            else
                await Dispatcher.InvokeAsync(ShowNoConnection);

            await Task.Delay(TimeSpan.FromSeconds(Math.Max(1, _cfg.PollSeconds)));
        }
    }

    private static void WriteHeartbeat()
    {
        try
        {
            File.WriteAllText(HeartbeatPath(), DateTime.Now.ToString("O"));
        }
        catch { }
    }

    /// <summary>
    /// Общий путь heartbeat — одинаковый для агента (пользователь) и сторожа (SYSTEM).
    /// %TEMP% у них разный, поэтому берём общий Public\Documents.
    /// </summary>
    private static string HeartbeatPath()
    {
        var dir = Environment.GetFolderPath(Environment.SpecialFolder.CommonDocuments);
        if (string.IsNullOrEmpty(dir)) dir = Path.GetTempPath();
        return Path.Combine(dir, "gcagent.heartbeat");
    }

    private enum Screen { None, Lock, NoConnection, Launcher, Password }
    private Screen _screen = Screen.None;

    private void SetScreen(Screen s)
    {
        if (s == _screen) return; // не перерисовываем, если экран тот же (иначе сбрасывается ввод)
        _screen = s;
        switch (s)
        {
            case Screen.Lock: RenderLock(); break;
            case Screen.NoConnection: RenderNoConnection(); break;
            case Screen.Launcher: RenderLauncher(); break;
            case Screen.Password: RenderPassword(); break;
        }
    }

    private void ApplyStatus(AgentStatus s)
    {
        _status = s;
        _hasData = true;
        UpdateSidebar();

        // фон обновился на сервере — перезагружаем
        if (s.BackgroundVersion != _bgVersion)
        {
            _bgVersion = s.BackgroundVersion;
            _ = LoadBackgroundAsync();
        }

        // выполняем команды сервера
        foreach (var cmd in (s.Commands ?? new List<PendingCommand>()))
        {
            ExecuteCommand(cmd.Kind);
            _ = _client.AckCommandAsync(cmd.Id, "done");
        }

        // сообщения от админа
        if (s.Messages != null && s.Messages.Count > 0)
        {
            var last = s.Messages[s.Messages.Count - 1];
            ShowMessage(last.Message);
            foreach (var m in s.Messages)
                _ = _client.AckMessageAsync(m.Id);
        }

        // выход из режима обслуживания (сессия возобновлена или завершена)
        if (_maintenance && !s.Paused)
        {
            _maintenance = false;
            Show();
            Activate();
        }

        var screen = _pwMode ? Screen.Password : (s.Locked ? Screen.Lock : Screen.Launcher);
        SetScreen(screen);

        // уведомление о малом остатке
        var low = _cfg.LowThresholdMinutes * 60;
        if (screen == Screen.Launcher && s.RemainingSeconds > 0 && s.RemainingSeconds <= low)
        {
            if (!_lastNotified)
            {
                _lastNotified = true;
                PillText.Text = $"Осталось {low / 60} минут";
                Pill.Visibility = Visibility.Visible;
            }
        }
        else
        {
            _lastNotified = false;
            Pill.Visibility = Visibility.Collapsed;
        }
    }

    private void ShowNoConnection()
    {
        _hasData = false;
        if (_pwMode) { SetScreen(Screen.Password); return; }
        SetScreen(Screen.NoConnection);
    }

    private void RenderNoConnection()
    {
        ShowShell(topmost: false);
        LockView.Visibility = Visibility.Visible;
        LauncherView.Visibility = Visibility.Collapsed;
        PasswordView.Visibility = Visibility.Collapsed;
        LockTitle.Text = "НЕТ СВЯЗИ С СЕРВЕРОМ";
        LockCode.Text = "";
        LockHint.Text = "Ctrl+Shift+F12 — выход в Windows";
    }

    private void RenderLock()
    {
        ShowShell(topmost: true);
        LockView.Visibility = Visibility.Visible;
        LauncherView.Visibility = Visibility.Collapsed;
        PasswordView.Visibility = Visibility.Collapsed;
        Pill.Visibility = Visibility.Collapsed;
        LockTitle.Text = "ВРЕМЯ ИСТЕКЛО";
        LockCode.Text = $"Зона: {_status.Zone}   |   ПК: {_status.PcName}";
        LockHint.Text = "Обратитесь к администратору";
    }

    private void RenderLauncher()
    {
        ShowShell(topmost: false);
        LockView.Visibility = Visibility.Collapsed;
        LauncherView.Visibility = Visibility.Visible;
        PasswordView.Visibility = Visibility.Collapsed;
        FooterInfo.Text = $"PC: {_status.PcName}  |  КОД: {_status.PcId}";
    }

    private void RenderPassword()
    {
        ShowShell(topmost: true);
        LockView.Visibility = Visibility.Collapsed;
        LauncherView.Visibility = Visibility.Collapsed;
        PasswordView.Visibility = Visibility.Visible;
    }

    private void ShowShell(bool topmost)
    {
        if (topmost)
        {
            if (!Topmost) Topmost = true;
            if (WindowState != WindowState.Maximized) WindowState = WindowState.Maximized;
        }
        else
        {
            if (Topmost) Topmost = false; // чтобы полноэкранная игра была поверх лаунчера
        }
    }

    private void OpenPassword()
    {
        _pwMode = true;
        PasswordError.Text = "";
        PasswordBox.Clear();
        SetScreen(Screen.Password);
        PasswordBox.Focus();
    }

    private void PasswordBox_KeyDown(object sender, System.Windows.Input.KeyEventArgs e)
    {
        if (e.Key == System.Windows.Input.Key.Enter)
            OkButton_Click(null, null);
    }

    private async void OkButton_Click(object sender, RoutedEventArgs e)
    {
        var pass = PasswordBox.Password;
        if (string.IsNullOrEmpty(pass)) { PasswordError.Text = "Введите пароль"; return; }

        // 1) локальный аварийный пароль (работает без сервера)
        if (!string.IsNullOrEmpty(_cfg.LocalAdminPassword) && pass == _cfg.LocalAdminPassword)
        {
            EnterMaintenance();
            return;
        }

        // 2) проверка на сервере
        var ok = await _client.ExitAuthAsync(pass);
        if (ok) EnterMaintenance();
        else PasswordError.Text = string.IsNullOrEmpty(_cfg.LocalAdminPassword)
            ? "Локальный пароль не задан в agent.json. Введите пароль администратора сервера."
            : "Неверный пароль или нет связи";
    }

    private void EnterMaintenance()
    {
        _pwMode = false;
        LockView.Visibility = Visibility.Collapsed;
        LauncherView.Visibility = Visibility.Collapsed;
        PasswordView.Visibility = Visibility.Collapsed;
        Pill.Visibility = Visibility.Collapsed;
        Hide();
        try { Process.Start(new ProcessStartInfo("explorer.exe") { UseShellExecute = true }); } catch { }
    }

    private async void GameButton_Click(object sender, RoutedEventArgs e)
    {
        if (_launching) return;
        if (sender is System.Windows.Controls.Button b && b.Tag is GameTileViewModel vm)
            await LaunchGameAsync(vm.Game);
    }

    private async Task LaunchGameAsync(GameConfig g)
    {
        _launching = true;
        LoadingText.Text = "Запуск: " + g.Name;
        LoadingOverlay.Visibility = Visibility.Visible;
        try
        {
            Process proc = null;
            var psi = new ProcessStartInfo
            {
                FileName = g.Command,
                UseShellExecute = true
            };
            if (!string.IsNullOrEmpty(g.Args)) psi.Arguments = g.Args;
            try { proc = Process.Start(psi); } catch { }

            if (proc == null || g.Command.StartsWith("steam://", StringComparison.OrdinalIgnoreCase))
            {
                // steam:// или shell-команда — процесс игры отследить сложно, ждём фиксированно
                await Task.Delay(4000);
            }
            else
            {
                // прямой exe — ждём появления окна (или завершения/таймаута)
                var sw = Stopwatch.StartNew();
                while (sw.ElapsedMilliseconds < 15000)
                {
                    try
                    {
                        proc.Refresh();
                        if (proc.HasExited) break;
                        if (proc.MainWindowHandle != IntPtr.Zero) break;
                    }
                    catch { break; }
                    await Task.Delay(250);
                }
            }
        }
        catch { }
        finally
        {
            LoadingOverlay.Visibility = Visibility.Collapsed;
            _launching = false;
        }
    }

    private void NvidiaBtn_Click(object sender, RoutedEventArgs e)
    {
        // 1) явная команда из agent.json (если задана)
        if (!string.IsNullOrEmpty(_cfg?.NvidiaCommand) && TryLaunch(_cfg.NvidiaCommand))
            return;

        // 2) nvcplui.exe через App Paths / PATH
        if (TryLaunch("nvcplui.exe")) return;

        // 3) классический путь драйвера
        if (TryLaunch(Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.ProgramFiles),
            @"NVIDIA Corporation\Control Panel Client\nvcplui.exe"))) return;

        // 4) System32
        if (TryLaunch(Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.System),
            "nvcplui.exe"))) return;

        // Панель NVIDIA не найдена автоматически — используйте nvidia_command в agent.json
        Log.Write("NVIDIA: панель не найдена. Задайте nvidia_command в agent.json");
    }

    private static bool TryLaunch(string file, string args = null)
    {
        try
        {
            var psi = new ProcessStartInfo(file) { UseShellExecute = true };
            if (args != null) psi.Arguments = args;
            if (Path.IsPathRooted(file) && !File.Exists(file)) return false;
            Process.Start(psi);
            return true;
        }
        catch
        {
            return false;
        }
    }

    // ===== Кастомный Alt+Tab =====

    private void InstallAltTabHook()
    {
        try
        {
            _hookProc = AltTabHookCallback;
            _hook = SetWindowsHookEx(WH_KEYBOARD_LL, _hookProc, IntPtr.Zero, 0);
        }
        catch { }
    }

    private void UninstallAltTabHook()
    {
        try
        {
            if (_hook != IntPtr.Zero) { UnhookWindowsHookEx(_hook); _hook = IntPtr.Zero; }
        }
        catch { }
    }

    private IntPtr AltTabHookCallback(int nCode, IntPtr wParam, IntPtr lParam)
    {
        if (nCode >= 0)
        {
            int vk = Marshal.ReadInt32(lParam);
            int msg = (int)wParam;
            bool down = msg == WM_KEYDOWN || msg == WM_SYSKEYDOWN;
            bool up = msg == WM_KEYUP || msg == WM_SYSKEYUP;

            if (vk == VK_TAB && down && (GetAsyncKeyState(VK_MENU) & 0x8000) != 0)
            {
                bool reverse = (GetAsyncKeyState(VK_LSHIFT) & 0x8000) != 0 || (GetAsyncKeyState(VK_RSHIFT) & 0x8000) != 0;
                Dispatcher.BeginInvoke(() => OnAltTab(reverse));
                return (IntPtr)1; // глотаем системный Alt+Tab
            }

            if (up && (vk == VK_MENU || vk == VK_LMENU || vk == VK_RMENU) && _switcherVisible)
            {
                Dispatcher.BeginInvoke(ActivateSelection);
            }
        }
        return CallNextHookEx(_hook, nCode, wParam, lParam);
    }

    private void OnAltTab(bool reverse)
    {
        if (!_switcherVisible) ShowSwitcher();
        else CycleSwitcher(reverse);
    }

    private void ShowSwitcher()
    {
        var wins = EnumerateWindows();
        if (wins.Count == 0) return;
        _windows = wins;
        _selIndex = 0;
        AltTabList.ItemsSource = wins.Select(w => new WinCardViewModel { Title = w.Title, Icon = w.Icon }).ToList();
        UpdateAltTabSelection();
        _switcherPrevTopmost = Topmost;
        Topmost = true;
        Activate();
        AltTabOverlay.Visibility = Visibility.Visible;
        _switcherVisible = true;
    }

    private void CycleSwitcher(bool reverse)
    {
        if (_windows == null || _windows.Count == 0) return;
        _selIndex += reverse ? -1 : 1;
        if (_selIndex < 0) _selIndex = _windows.Count - 1;
        if (_selIndex >= _windows.Count) _selIndex = 0;
        UpdateAltTabSelection();
    }

    private void UpdateAltTabSelection()
    {
        if (AltTabList.ItemsSource is List<WinCardViewModel> cards)
            for (int i = 0; i < cards.Count; i++)
                cards[i].Selected = i == _selIndex;
    }

    private void ActivateSelection()
    {
        if (!_switcherVisible) return;
        var hwnd = (_windows != null && _selIndex >= 0 && _selIndex < _windows.Count) ? _windows[_selIndex].Hwnd : IntPtr.Zero;
        HideAltTab(); // сначала убираем лаунчер с topmost
        if (hwnd != IntPtr.Zero)
            FocusWindow(hwnd); // затем поднимаем выбранное окно наверх
    }

    private static void FocusWindow(IntPtr hwnd)
    {
        try
        {
            if (hwnd == IntPtr.Zero) return;
            if (IsIconic(hwnd)) ShowWindow(hwnd, SW_RESTORE);
            ShowWindow(hwnd, SW_SHOW);
            // явно ставим окно на самый верх z-порядка (поверх лаунчера)
            SetWindowPos(hwnd, HWND_TOP, 0, 0, 0, 0, SWP_NOMOVE | SWP_NOSIZE);

            uint targetPid;
            uint targetThread = GetWindowThreadProcessId(hwnd, out targetPid);
            uint ourThread = GetCurrentThreadId();

            if (targetThread != ourThread)
            {
                AttachThreadInput(ourThread, targetThread, true);
                SetForegroundWindow(hwnd);
                AttachThreadInput(ourThread, targetThread, false);
            }
            else
            {
                SetForegroundWindow(hwnd);
            }
            SetForegroundWindow(hwnd);
        }
        catch { }
    }

    private void HideAltTab()
    {
        AltTabOverlay.Visibility = Visibility.Collapsed;
        Topmost = _switcherPrevTopmost;
        _switcherVisible = false;
        _windows = null;
    }

    private List<WinInfo> EnumerateWindows()
    {
        var result = new List<WinInfo>();
        var myHwnd = new WindowInteropHelper(this).Handle;
        EnumWindows((hwnd, lParam) =>
        {
            if (hwnd == myHwnd) return true;
            if (!IsWindowVisible(hwnd)) return true;
            int exStyle = GetWindowLong(hwnd, GWL_EXSTYLE);
            if ((exStyle & WS_EX_TOOLWINDOW) != 0) return true;
            var sb = new StringBuilder(256);
            GetWindowText(hwnd, sb, sb.Capacity);
            var title = sb.ToString().Trim();
            if (title.Length == 0) return true;
            if (title == "Program Manager" || title == "PlayCore") return true;
            result.Add(new WinInfo { Hwnd = hwnd, Title = title, Icon = GetWindowIcon(hwnd) });
            return true;
        }, IntPtr.Zero);
        return result;
    }

    private static ImageSource GetWindowIcon(IntPtr hwnd)
    {
        try
        {
            IntPtr hIcon = SendMessage(hwnd, WM_GETICON, (IntPtr)1, IntPtr.Zero); // ICON_BIG
            if (hIcon == IntPtr.Zero) hIcon = SendMessage(hwnd, WM_GETICON, (IntPtr)2, IntPtr.Zero); // ICON_SMALL2
            if (hIcon == IntPtr.Zero) hIcon = GetClassLongPtr(hwnd, GCL_HICON);
            if (hIcon == IntPtr.Zero) hIcon = GetClassLongPtr(hwnd, GCL_HICONSM);
            if (hIcon != IntPtr.Zero)
                return Imaging.CreateBitmapSourceFromHIcon(hIcon, Int32Rect.Empty, BitmapSizeOptions.FromEmptyOptions());
        }
        catch { }
        return null;
    }

    private class WinInfo
    {
        public IntPtr Hwnd;
        public string Title;
        public ImageSource Icon;
    }

    private void ExecuteCommand(string kind)
    {
        switch (kind)
        {
            case "close_apps": CloseApps(); break;
            case "shutdown": RunShutdown("/s"); break;
            case "reboot": RunShutdown("/r"); break;
            case "exit_to_admin": _maintenance = true; EnterMaintenance(); break;
        }
    }

    private void ShowMessage(string text)
    {
        MessageText.Text = text;
        MessageOverlay.Visibility = Visibility.Visible;
        Topmost = true; // поверх игры
        try { System.Media.SystemSounds.Exclamation.Play(); } catch { }
    }

    private void MessageOk_Click(object sender, RoutedEventArgs e)
    {
        MessageOverlay.Visibility = Visibility.Collapsed;
        if (_screen == Screen.Launcher) Topmost = false;
    }

    private static void RunShutdown(string flag)
    {
        try
        {
            Process.Start(new ProcessStartInfo("shutdown.exe", $"{flag} /t 0 /f")
            {
                UseShellExecute = false,
                CreateNoWindow = true
            });
        }
        catch { }
    }

    private static readonly HashSet<string> ProtectedProcesses = new(StringComparer.OrdinalIgnoreCase)
    {
        "explorer", "dwm", "csrss", "wininit", "winlogon", "services", "lsass", "svchost",
        "smss", "fontdrvhost", "sihost", "taskhostw", "ctfmon", "runtimebroker",
        "shellexperiencehost", "startmenuexperiencehost", "textinputhost", "applicationframehost",
        "clubshell", "clubagent.service", "faceit", "esportal"
    };

    private static void CloseApps()
    {
        var mySession = Process.GetCurrentProcess().SessionId;
        foreach (var p in Process.GetProcesses())
        {
            try
            {
                if (p.Id == Process.GetCurrentProcess().Id) continue;
                if (p.SessionId != mySession) continue;
                if (ProtectedProcesses.Contains(p.ProcessName)) continue;
                p.Kill();
            }
            catch { }
        }
    }
}

public class GameTileViewModel : System.ComponentModel.INotifyPropertyChanged
{
    public Club.Core.GameConfig Game { get; set; }
    public string Name => Game.Name;
    public ImageSource Icon { get; set; }

    private ImageSource _cover;
    public ImageSource Cover
    {
        get => _cover;
        set
        {
            _cover = value;
            PropertyChanged?.Invoke(this, new System.ComponentModel.PropertyChangedEventArgs(nameof(Cover)));
        }
    }

    public event System.ComponentModel.PropertyChangedEventHandler PropertyChanged;
}

public class ProductVm
{
    public long Id { get; set; }
    public string Name { get; set; } = "";
    public string PriceLabel { get; set; } = "";
}

public class CategoryVm
{
    public string Name { get; set; } = "";
    public string PriceLabel { get; set; } = "";
}

public class WinCardViewModel : System.ComponentModel.INotifyPropertyChanged
{
    public string Title { get; set; }
    public ImageSource Icon { get; set; }

    private bool _selected;
    public bool Selected
    {
        get => _selected;
        set
        {
            _selected = value;
            PropertyChanged?.Invoke(this, new System.ComponentModel.PropertyChangedEventArgs(nameof(Selected)));
        }
    }

    public event System.ComponentModel.PropertyChangedEventHandler PropertyChanged;
}
