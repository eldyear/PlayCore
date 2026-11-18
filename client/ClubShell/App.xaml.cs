using System;
using System.Threading;
using System.Windows;

namespace Club.Shell;

public partial class App : Application
{
    private Mutex _mutex;

    protected override void OnStartup(StartupEventArgs e)
    {
        // режимы установки/отката — без запуска окна
        var exePath = Environment.ProcessPath ?? "";
        if (e.Args.Contains("-install")) { Installer.Install(exePath); Shutdown(); return; }
        if (e.Args.Contains("-uninstall")) { Installer.Uninstall(); Shutdown(); return; }

        // только один экземпляр агента
        _mutex = new Mutex(true, "Local\\GCAgent_SingleInstance", out bool createdNew);
        if (!createdNew)
        {
            Shutdown();
            return;
        }

        // ловим необработанные исключения и пишем в лог
        DispatcherUnhandledException += (s, ex) =>
        {
            Log.Write("UI EXCEPTION: " + ex.Exception);
            ex.Handled = true; // не роняем приложение — показываем, что пошло не так
        };
        AppDomain.CurrentDomain.UnhandledException += (s, ex) =>
        {
            Log.Write("UNHANDLED: " + ex.ExceptionObject);
        };

        base.OnStartup(e);
    }
}
