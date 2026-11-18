using System;
using System.IO;
using System.Net.Http;
using System.Threading.Tasks;
using System.Windows.Media.Imaging;

namespace Club.Shell;

/// <summary>
/// Скачивает и кэширует обложки игр Steam (публичный CDN, без API-ключа).
/// Кэш: covers/{appid}.jpg рядом с exe.
/// </summary>
public static class CoverCache
{
    private static readonly HttpClient Http = new() { Timeout = TimeSpan.FromSeconds(10) };
    private static string Dir => Path.Combine(AppContext.BaseDirectory, "covers");

    public static async Task<BitmapImage> GetAsync(long appId)
    {
        var path = Path.Combine(Dir, appId + ".jpg");
        try
        {
            if (File.Exists(path))
                return LoadBitmap(path);

            Directory.CreateDirectory(Dir);
            var url = $"https://cdn.cloudflare.steamstatic.com/steam/apps/{appId}/library_600x900.jpg";
            var bytes = await Http.GetByteArrayAsync(url);
            await File.WriteAllBytesAsync(path, bytes);
            return LoadBitmap(path);
        }
        catch
        {
            return null;
        }
    }

    private static BitmapImage LoadBitmap(string path)
    {
        var bmp = new BitmapImage();
        bmp.BeginInit();
        bmp.CacheOption = BitmapCacheOption.OnLoad;
        bmp.CreateOptions = BitmapCreateOptions.IgnoreImageCache;
        bmp.UriSource = new Uri(path);
        bmp.EndInit();
        bmp.Freeze();
        return bmp;
    }
}
