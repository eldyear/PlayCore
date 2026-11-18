using System;
using System.IO;

namespace Club.Shell;

/// <summary>Простой файловый лог (agent.log рядом с exe).</summary>
public static class Log
{
    private static readonly object Locker = new();

    public static void Write(string message)
    {
        try
        {
            var line = DateTime.Now.ToString("yyyy-MM-dd HH:mm:ss") + "  " + message + Environment.NewLine;
            lock (Locker)
            {
                try
                {
                    File.AppendAllText(Path.Combine(AppContext.BaseDirectory, "agent.log"), line);
                }
                catch
                {
                    // exe-каталог недоступен на запись (например, Program Files) — пишем в Public\Documents
                    var dir = Environment.GetFolderPath(Environment.SpecialFolder.CommonDocuments);
                    File.AppendAllText(Path.Combine(dir, "ClubAgent.agent.log"), line);
                }
            }
        }
        catch
        {
            // лог недоступен — молча пропускаем
        }
    }
}
