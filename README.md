# Slang Discovery Engine — Windows Desktop (v6)

Bu paket, **gerçek bir Windows x64 `.exe` dosyası ve çift tıklanabilir `Setup.exe`** içerir. 1980'lerin sonu / 1990'lar idari terminal estetiğinden ve Windows XP'nin kare pencerelerinden esinlenen **kırmızı / zeytin / krem** tasarıma sahiptir. Logo bulunmaz.

## Hemen kurulum (Windows 10/11 x64)

1. ZIP'i bir klasöre çıkarın.
2. **`Setup.exe` dosyasına çift tıklayın.** Yönetici yetkisi gerektirmez; `%LOCALAPPDATA%\Programs\SlangDiscoveryEngine\SlangDiscovery.exe` içine kurulum yapar.
3. Masaüstünde ve Başlat menüsünde **Slang Discovery Engine** kısayolu oluşturulur.
4. Kısayola çift tıklayın. Program **127.0.0.1** üzerindeki yerel arayüzü varsayılan tarayıcıda açar. İnternet yalnızca Reddit'e API isteği gönderilirken gereklidir.
5. Önce **DEMO YÜKLE** butonuyla deneyin. Reddit için **Ayarlar > Reddit Ayarları** içinden Client ID, Client Secret, User-Agent, subreddit ve limitleri kaydedebilirsiniz. **ⓘ REDDIT API NASIL ALINIR?** rehberini okuyun.
6. Tek başına kullanmak isterseniz `SlangDiscovery.exe` dosyasını doğrudan çalıştırabilirsiniz. Python ve Go kurulumu gerektirmez.

> Windows SmartScreen: İmzalanmamış bağımsız derlenmiş uygulamalarda uyarı çıkabilir. Kaynak kodu pakettedir; yalnızca güvendiğiniz dosyaları çalıştırın. Bu ortamda Windows üzerinde GUI/kurulum uçtan uca çalıştırılamamıştır.

## Özellikler

- API ayarlarını program içinde değiştirme; Reddit OAuth (client credentials) ile yalnızca yetkili veri erişimi.
- Reddit API alma yardım penceresi; güncel başvuru / politikalar bağlantıları.
- Windows DPAPI ile **client secret** şifreleme; şifre tekrar görüntülenmez, açık metin olarak kaydedilmez.
- Reddit gönderileri, sınırlı doğrudan yorumlar ve subreddit keşfi.
- Reddit olmadan demo veya lisanslı/kullanım izni olan **JSONL** dosyalarından içe aktarma.
- Gömülü CMU Pronouncing Dictionary referansı (117.249 sözcük) ile sıradan İngilizce kelimeleri eleme.
- Yeni/alşılmadık kelime, çok kelimeli ifade ve basit bağlam değişimi hipotezleri.
- Aday arama, filtreleme, değerlendirme ve kanıt bağlantısı; haftalık normalize trend; CSV dışa aktarma.
- Yerel kayıt ve şifreleme, kullanıcı adı yerine salted hash, 90 günlük varsayılan saklama; maksimum 15.000 belge.
- İnternet olmadan çalışan web temelli masaüstü konsolu, logo içermeyen Sovyet dönemi esintili Windows XP görünümü.

### JSONL veri içe aktarma biçimi

Her satır ayrı JSON nesnesi olmalıdır:

```json
{"id":"sample-001","body":"bro this glorpmaxxing term is everywhere","subreddit":"community","created_utc":1790000000,"author":"example-user","source":"licensed-export","url":"https://example.org/post/1"}
```

`author` değeri diske açık biçimde kaydedilmez; rastgele salt ile hashlenir. `author_hash` alanını kendiniz verirseniz doğrudan kullanılır. Kullanım izni olan metinleri yükleyin. İçe aktarılan veri `import` etiketiyle gösterilir (Reddit etiketi verilmez).

## Reddit API alma

**Ayarlar > ⓘ REDDIT API NASIL ALINIR?** butonunda adım adım güncel rehber bulunur. Bu işleyiş Reddit'in iznine tabidir: yeni geliştiricilerden başvuru istenebilir. Geleneksel OAuth client ID/secret herkese otomatik verilmeyebilir. Başvuru ve şartları kontrol edin:

- https://support.reddithelp.com/hc/en-us/articles/42728983564564-Responsible-Builder-Policy
- https://support.reddithelp.com/hc/en-us/requests/new?ticket_form_id=14868593862164
- https://www.reddit.com/prefs/apps
- https://redditinc.com/policies/data-api-terms
- https://www.reddit.com/r/redditdev/comments/1wubcvf/moving_data_api_apps_to_the_developer_platform/

Reddit'in 30 Eylül 2026 duyurusunda 2027'de Data API erişiminin Developer Platform'a taşınması planı açıklanmıştır. İzinli eski API uyumluluğu zamanla değişebilir. API erişim onayı, ham metin toplama ve model eğitimi haklarını tek başına garanti etmez.

## Kaynak kodları ve yeniden derleme

- **`app/`**: Go 1.23 stdlib, Python gerektirmeyen Windows programı (Reddit istemcisi, JSON depolama, analiz, yerel HTTP/UI, DPAPI).
- **`app/web/`**: HTML/CSS/JS arayüz, temasız bütünleşik tasarım.
- **`installer/`**: Windows masaüstü/başlat menüsü `.lnk` kısayolunu oluşturan bağımsız kurulum EXE kaynak kodu.
- **`source-python-v5/`**: önceki v5 araştırma motoru (isteğe bağlı daha ileri NLP deneyleri). **Masaüstü EXE içine gömülü değildir** ve ayrıca çalıştırılmalıdır.
- **`build_windows.ps1`**: Windows üzerinde Go 1.23 ile EXE ve Setup'ı yeniden derler.
- **`docs/`**: ekran önizlemeleri, lisans ve test raporu.

## Önemli sınırlar

- **Bu uygulama bir slang aday bulucudur; doğrulanmış slang anlamı sözlüğü değildir.** Bağlam değişimi bir hipotezdir; gerçek anlam değişiminin ispatı değildir.
- Kullanıcı adları hashlenir ama kaynak içerikleri ve bağlantılar saklanır; kişisel veri/Reddit silme yükümlülüklerini ayrıca gözden geçirin. Bu sürümde Reddit içeriklerinin silinme senkronizasyonu otomatik yapılmaz.
- Veri çekimi yeni postların sınırlı bir kesitinden yapılır, tüm tarihsel Reddit kapsamını temsil etmez; rate-limit ve erişim onayları gereklidir.
- Windows programındaki algoritma Go'da taşınabilir, daha hafif bir keşif motorudur; önceki Python v5'teki SentenceTransformers / TF-IDF deneyleri exe içinde çalıştırılmaz.
- Kurulumun Windows'ta gerçek çift tıklama ve kısayol oluşturma aşamaları bu Linux ortamında koşturulamamış; derleyici her iki EXE'yi Windows PE64 biçiminde üretebilmiştir.

## Kaldırma

Masaüstü ve Başlat menüsü `Slang Discovery Engine` kısayollarını silin. `%LOCALAPPDATA%\Programs\SlangDiscoveryEngine` klasörünü kaldırın. Verileri de kaldırmak için **yedekledikten sonra** `%LOCALAPPDATA%\SlangDiscoveryEngine` klasörünü silin (gizli anahtar bu klasörde DPAPI korumalıdır).

## Lisans bildirimi

Referans sözcük listesi Carnegie Mellon University CMUdict verisinden türetilmiştir. Lisans metni `docs/CMUDICT-LICENSE.txt` dosyasındadır; redistribüsyon yükümlülükleri bu metne tabidir. Bu program Reddit, Microsoft veya herhangi bir resmî devlet kurumu ile bağlantılı değildir; herhangi bir resmî logo kullanılmamıştır.
