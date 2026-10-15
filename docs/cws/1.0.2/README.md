# Store screenshots — расширение 1.0.2

Три финальных PNG после polish popup, 640×400: поддерживаемая страница YouTube, обнаруженный MP4 на публичной W3Schools HTML5 video page и открытый компактный privacy overlay с клавиатурным фокусом. Это настоящие снимки action popup отдельного Chrome profile, без mockup, DevTools, персональных данных и служебных баннеров. Chrome Page.captureScreenshot снимает область 320×200 CSS px в масштабе 2×; фон рисует сам Chrome. Текст, данные и элементы popup не дорисовывались. Старый комплект до финального polish устарел.

Runtime-файлы получены штатным production exporter 1.0.2 и сверяются с итоговым ZIP из clean commit. Для связи тестового Chrome с установленным host только в отдельную unpacked копию manifest добавлен repository dev public key; ZIP не менялся. Такая проверка не заменяет acceptance обновления, установленного из Store после публикации.

`node scripts/cws-store-assets.mjs` проверяет размеры и экспортирует эти PNG вместе с актуальной packaged icon 128×128 в `dist/release/1.0.2/store/`. Временные captures и test profiles в Git не входят.

Требования CWS проверены 15 октября 2026 года: [официальные правила изображений](https://developer.chrome.com/docs/webstore/images). Допускается от одного до пяти screenshots, PNG/JPEG, 640×400 либо 1280×800. Старые screenshots с прежним popup нужно заменить всеми тремя файлами из этой папки; существующие promo tiles не требуют замены, если не содержат старого интерфейса.
