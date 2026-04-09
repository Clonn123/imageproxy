# imageproxy

[![GoDoc](https://img.shields.io/badge/godoc-reference-blue)](https://pkg.go.dev/willnorris.com/go/imageproxy)
[![Test Status](https://github.com/willnorris/imageproxy/workflows/tests/badge.svg)](https://github.com/willnorris/imageproxy/actions?query=workflow%3Atests)
[![Test Coverage](https://codecov.io/gh/willnorris/imageproxy/branch/main/graph/badge.svg)](https://codecov.io/gh/willnorris/imageproxy)
[![CII Best Practices](https://bestpractices.coreinfrastructure.org/projects/2611/badge)](https://bestpractices.coreinfrastructure.org/projects/2611)

imageproxy — это кэширующий прокси-сервер изображений, написанный на Go. Он предоставляет:

- базовые операции с изображениями: изменение размера, обрезка и поворот
- контроль доступа с помощью списка разрешённых хостов или подписанных запросов (HMAC-SHA256)
- поддержку форматов jpeg, png, webp (только декодирование), tiff и gif (включая анимированные gif)
- кэширование в памяти, на диске или в Amazon S3, Google Cloud Storage, Azure Storage или Redis
- простое развёртывание, поскольку это чистая реализация на Go

Я использую его в основном для динамического изменения размера изображений, размещённых на моём сайте (подробнее в [this post][]). Но вы также можете включить подпись запросов и использовать его как SSL-прокси для удалённых изображений, аналогично [atmos/camo][], с дополнительными возможностями трансформации изображений.

Я стараюсь поддерживать совместимость imageproxy с двумя последними мажорными версиями Go. Кроме того, я отслеживаю минимальную версию Go, с которой всё ещё работает проект (в настоящее время go1.18), но это может измениться в любой момент. Список версий Go, против которых выполняются тесты, можно увидеть в [.github/workflows/tests.yml][].

## Структура URL

URL-ы imageproxy имеют формат `http://localhost/{options}/{remote_url}`.

При использовании настроенных префиксов хранилища URL принимает вид
`http://localhost/{storage}/{remote_path}?x=300&y=200`.
Сегмент `{storage}` выбирает настроенный базовый URL, оставшаяся часть пути
трактуется как путь к файлу в хранилище, а query-параметры управляют
трансформациями изображения.

### Опции

Опции позволяют выполнять обрезку, изменение размера, поворот, отражение, цифровые подписи и ещё несколько действий. Опции задаются в виде списка параметров, разделённых запятыми, и могут быть указаны в любом порядке. Дублирующиеся параметры перезаписывают предыдущие значения.

Полный список доступных опций см. в <https://pkg.go.dev/willnorris.com/go/imageproxy#ParseOptions>.

### Удалённый URL

URL исходного изображения указывается остатком пути. Он может быть указан в явном виде без кодирования, в percent-encoding (URL-encoded) или в base64 (URL-safe, без padding).

Если кодирование не используется, любая query-строка в прокси-URL трактуется как часть удалённого URL. Например, при прокси-URL `http://localhost/x/http://example.com/?id=1` удалённый URL будет `http://example.com/?id=1`.

При использовании percent-encoding полный URL должен быть закодирован. Любая query-строка в прокси-URL НЕ включается в удалённый URL. Percent-encoded URL должны быть абсолютными; они не могут быть относительными URL, используемыми с базовым URL. Например: `http://localhost/x/http%3A%2F%2Fexample.com%2F%3Fid%3D1`.

При использовании base64 кодируется весь URL. Любая query-строка в прокси-URL НЕ включается в удалённый URL. Base64-кодированные URL могут быть относительными и использованы с базовым URL. Например: `http://localhost/x/aHR0cDovL2V4YW1wbGUuY29tLz9pZD0x`.

### Примеры

Ниже приведены живые примеры, демонстрирующие различные опции на [этом исходном изображении][small-things], которое имеет размер 1024×678.

| Options                | Meaning                                                    | Image                                                                                                                                                                                                                                                                                                |
| ---------------------- | ---------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 200x                   | 200px по ширине, пропорциональная высота                   | <a href="https://willnorris.com/api/imageproxy/200x/https://willnorris.com/images/imageproxy/small-things.jpg"><img src="https://willnorris.com/api/imageproxy/200x/https://willnorris.com/images/imageproxy/small-things.jpg" alt="200x"></a>                                                       |
| x0.15                  | 15% от оригинальной высоты, пропорциональная ширина        | <a href="https://willnorris.com/api/imageproxy/x0.15/https://willnorris.com/images/imageproxy/small-things.jpg"><img src="https://willnorris.com/api/imageproxy/x0.15/https://willnorris.com/images/imageproxy/small-things.jpg" alt="x0.15"></a>                                                    |
| 100x150                | 100×150 пикселей, обрезка по необходимости                 | <a href="https://willnorris.com/api/imageproxy/100x150/https://willnorris.com/images/imageproxy/small-things.jpg"><img src="https://willnorris.com/api/imageproxy/100x150/https://willnorris.com/images/imageproxy/small-things.jpg" alt="100x150"></a>                                              |
| 100                    | 100px квадрат, обрезка по необходимости                    | <a href="https://willnorris.com/api/imageproxy/100/https://willnorris.com/images/imageproxy/small-things.jpg"><img src="https://willnorris.com/api/imageproxy/100/https://willnorris.com/images/imageproxy/small-things.jpg" alt="100"></a>                                                          |
| 150,fit                | масштабировать, чтобы вписаться в 150px квадрат, без обрезки | <a href="https://willnorris.com/api/imageproxy/150,fit/https://willnorris.com/images/imageproxy/small-things.jpg"><img src="https://willnorris.com/api/imageproxy/150,fit/https://willnorris.com/images/imageproxy/small-things.jpg" alt="150,fit"></a>                                              |
| 100,r90                | 100px квадрат, поворот 90 градусов                         | <a href="https://willnorris.com/api/imageproxy/100,r90/https://willnorris.com/images/imageproxy/small-things.jpg"><img src="https://willnorris.com/api/imageproxy/100,r90/https://willnorris.com/images/imageproxy/small-things.jpg" alt="100,r90"></a>                                              |
| 100,fv,fh              | 100px квадрат, отражение по вертикали и горизонтали        | <a href="https://willnorris.com/api/imageproxy/100,fv,fh/https://willnorris.com/images/imageproxy/small-things.jpg"><img src="https://willnorris.com/api/imageproxy/100,fv,fh/https://willnorris.com/images/imageproxy/small-things.jpg" alt="100,fv,fh"></a>                                        |
| 200x,q60               | 200px по ширине, пропорциональная высота, качество 60%      | <a href="https://willnorris.com/api/imageproxy/200x,q60/https://willnorris.com/images/imageproxy/small-things.jpg"><img src="https://willnorris.com/api/imageproxy/200x,q60/https://willnorris.com/images/imageproxy/small-things.jpg" alt="200x,q60"></a>                                           |
| 200x,png               | 200px по ширине, конвертация в PNG                         | <a href="https://willnorris.com/api/imageproxy/200x,png/https://willnorris.com/images/imageproxy/small-things.jpg"><img src="https://willnorris.com/api/imageproxy/200x,png/https://willnorris.com/images/imageproxy/small-things.jpg" alt="200x,png"></a>                                           |
| cx175,cw400,ch300,100x | обрезать до 400×300 начиная с (175,0), масштабировать до 100px ширины | <a href="https://willnorris.com/api/imageproxy/cx175,cw400,ch300,100x/https://willnorris.com/images/imageproxy/small-things.jpg"><img src="https://willnorris.com/api/imageproxy/cx175,cw400,ch300,100x/https://willnorris.com/images/imageproxy/small-things.jpg" alt="cx175,cw400,ch300,100x"></a> |

Функцию [smart crop](https://pkg.go.dev/willnorris.com/go/imageproxy#hdr-Smart_Crop-ParseOptions) лучше всего видно при сравнении обрезок [этого изображения][judah-sheets] с включённой и выключенной опцией smart crop.

| Options    | Meaning                  | Image                                                                                                                                                                                                                                     |
| ---------- | ------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 150x300    | 150×300, стандартная обрезка | <a href="https://willnorris.com/api/imageproxy/150x300/https://judahnorris.com/images/judah-sheets.jpg"><img src="https://willnorris.com/api/imageproxy/150x300/https://judahnorris.com/images/judah-sheets.jpg" alt="200x400,sc"></a>    |
| 150x300,sc | 150×300, smart crop       | <a href="https://willnorris.com/api/imageproxy/150x300,sc/https://judahnorris.com/images/judah-sheets.jpg"><img src="https://willnorris.com/api/imageproxy/150x300,sc/https://judahnorris.com/images/judah-sheets.jpg" alt="200x400"></a> |

[judah-sheets]: https://judahnorris.com/images/judah-sheets.jpg

Трансформации также работают с анимированными gif. Вот [это исходное изображение][material-animation], уменьшенное до квадрата 200px и повернутое на 270 градусов:

[material-animation]: https://willnorris.com/images/imageproxy/material-animations.gif

<a href="https://willnorris.com/api/imageproxy/200,r270/https://willnorris.com/images/imageproxy/material-animations.gif"><img src="https://willnorris.com/api/imageproxy/200,r270/https://willnorris.com/images/imageproxy/material-animations.gif" alt="200,r270"></a>

## Начало работы

Установите пакет командой:

```sh
go install willnorris.com/go/imageproxy/cmd/imageproxy@latest
```

После установки убедитесь, что `$GOPATH/bin` находится в вашем `$PATH`, затем запустите прокси:

```sh
imageproxy
```

Это запустит прокси на порту 8080, без кэширования и без списка разрешённых хостов (то есть можно проксировать любые удалённые URL). Проверьте, перейдя по адресу <http://localhost:8080/500/https://octodex.github.com/images/codercat.jpg> — вы должны увидеть квадратно обрезанное изображение coder octocat размером 500px.

### Кэш

По умолчанию команда imageproxy не кеширует ответы, но кэш можно включить с помощью флага `-cache`. Поддерживаются следующие варианты:

- `memory` — использует LRU-кэш в памяти. По умолчанию ограничен 100 МБ. Чтобы настроить размер кэша или максимальный возраст кэшируемых элементов, используйте формат `memory:size:age`, где размер указан в мегабайтах, а возраст — в формате продолжительности. Например, `memory:200:4h` создаст кэш на 200 МБ с максимальным временем хранения 4 часа.
- директория на локальном диске (например, `/tmp/imageproxy`) — будет кэшировать изображения на диске
- s3 URL (например, `s3://region/bucket-name/optional-path-prefix`) — будет кэшировать изображения в Amazon S3. Для этого нужна роль IAM и профиль экземпляра с правами доступа к бакету или переменные окружения `AWS_ACCESS_KEY_ID` и `AWS_SECRET_KEY`. (Дополнительные методы загрузки учётных данных описаны в пакете aws-sdk-go session).

  Дополнительные параметры конфигурации ([подробно здесь][aws-options]) могут быть заданы в query-строке URL, что полезно при работе с S3-совместимыми сервисами:

  - "endpoint" — указать альтернативный API endpoint
  - "disableSSL" — установить в "1", чтобы отключить SSL при вызовах API
  - "s3ForcePathStyle" — установить в "1", чтобы принудительно использовать path-style addressing

  Например, при работе с [minio](https://minio.io), который не использует регионы, укажите фиктивный регион и кастомный endpoint:

  ```
  s3://fake-region/bucket/folder?endpoint=minio:9000&disableSSL=1&s3ForcePathStyle=1
  ```

  Аналогично для [Digital Ocean Spaces](https://www.digitalocean.com/products/spaces/), укажите фиктивный регион и соответствующий endpoint для вашего пространства:

  ```
  s3://fake-region/bucket/folder?endpoint=sfo2.digitaloceanspaces.com
  ```

- gcs URL (например, `gcs://bucket-name/optional-path-prefix`) — будет кэшировать в Google Cloud Storage. Аутентификация описана в документации Google про [Application Default Credentials](https://cloud.google.com/docs/authentication/production#providing_credentials_to_your_application).
- azure URL (например, `azure://container-name/`) — будет кэшировать в Azure Storage. Для этого требуются переменные окружения `AZURESTORAGE_ACCOUNT_NAME` и `AZURESTORAGE_ACCESS_KEY`.
- redis URL (например, `redis://hostname/`) — будет кэшировать на указанном redis-хосте. Полный синтаксис URL определяется [redis URI registration]. Вместо указания пароля в URI используйте переменную окружения `REDIS_PASSWORD`.

Например, чтобы кэшировать файлы на диске в `/tmp/imageproxy`:

```sh
imageproxy -cache /tmp/imageproxy
```

Перезагрузите [codercat URL][], затем проверьте содержимое `/tmp/imageproxy`. В поддиректориях должны появиться два файла: один для оригинального полноразмерного изображения, и один — для уменьшенной 500px версии.

Несколько кэшей можно указать, разделяя их пробелами или повторяя флаг `-cache`. Кэши будут созданы в иерархическом порядке (tiered). Обычно это нужно, чтобы поместить небольшой и быстрый кэш в памяти перед большим, но более медленным дисковым кэшем. Например, следующая конфигурация сначала проверяет память, затем gcs-бакет:

```sh
imageproxy -cache memory -cache gcs://my-bucket/
```

[tiered fashion]: https://pkg.go.dev/github.com/die-net/lrucache/twotier

#### Переопределение директив кэширования

По умолчанию imageproxy уважает директивы кэширования в заголовках ответа, включая время жизни кэша и явные инструкции **не кешировать** (например, `no-store` или `private`).

Вы можете принудительно заставить imageproxy кэшировать ответы, даже если они явно запрещают это, с помощью флага `-forceCache`. Однако это не рекомендуется в большинстве случаев.

Минимальное время кэширования можно задать флагом `-minCacheDuration`. Это продлит время хранения в кэше, если в заголовке ответа указано меньшее значение. Если флаг `-forceCache` не указан, это не повлияет на ответы с директивами `no-store` или `private`.

```sh
imageproxy -cache /tmp/imageproxy -minCacheDuration 5m
```

### Список разрешённых рефереров

Вы можете ограничить доступ к изображениям по значению заголовка HTTP `Referer`, что помогает предотвратить хотлинкинг. Включается так:

```sh
imageproxy -referrers example.com
```

Перезагрузите [codercat URL][], и вы должны получить сообщение об ошибке. Можно указать несколько хостов через запятую или использовать префикс `*.` для разрешения поддоменов.

### Списки разрешённых и запрещённых хостов

Вы можете ограничить удалённые хосты, с которых прокси будет загружать изображения, с помощью флагов `allowHosts` и `denyHosts`. Это полезно, например, чтобы ограничить прокси доступом только к вашим собственным хостам. Если вы хотите разрешить любые хосты, не указывайте эти флаги.

Попробуйте:

```sh
imageproxy -allowHosts example.com
```

Перезагрузите [codercat URL][], и вы увидите ошибку. Также можно выполнить:

```sh
imageproxy -denyHosts octodex.github.com
```

Если хост совпадает и с разрешённым, и с запрещённым списком, запрос будет отклонён.

Можно указывать несколько хостов через запятую, использовать `*.` для поддоменов или задавать сетевые блоки в формате CIDR (`127.0.0.0/8`) — это удобно для блокировки зарезервированных диапазонов вроде `127.0.0.0/8`, `192.168.0.0/16` и т.п.

### Список разрешённых Content-Type

Вы можете ограничить типы содержимого, которые проксируются, используя флаг `contentTypes`. По умолчанию установлен `image/*`, то есть imageproxy будет обрабатывать любые типы изображений. Можно задать несколько типов через запятую и использовать суффикс `*` для подстановки. Установите пустую строку, чтобы проксировать все запросы независимо от типа содержимого.

### Подписанные запросы

Вместо списка разрешённых хостов вы можете потребовать подписанные запросы. Это полезно, если вы не хотите поддерживать статический список хостов. Подписи генерируются с помощью HMAC-SHA256 по удалённому URL, затем результат кодируется в base64 URL-safe:

```
base64urlencode(hmac.New(sha256, <key>).digest(<remote_url>))
```

Ключ HMAC задаётся флагом `signatureKey`. Если значение флага начинается с `@`, оставшаяся часть интерпретируется как путь к файлу на диске, содержащему ключ.

Попробуйте:

```sh
imageproxy -signatureKey "secretkey"
```

Перезагрузите [codercat URL][], и вы увидите сообщение об ошибке. Затем загрузите [signed codercat URL][] (в котором присутствует опция [signature]) и убедитесь, что он открывается корректно.

Некоторые примеры кода для генерации подписей на разных языках есть в [docs/url-signing.md](/docs/url-signing.md). Можно указать несколько валидных ключей для поддержки ротации ключей, повторяя флаг `signatureKey` несколько раз или передавая список ключей через пробел. Чтобы использовать ключ с пробелом, загрузите ключ из файла через префикс `@`.

Если указаны и whitelist, и signatureKey, то запросы, соответствующие whitelist, не обязаны быть подписанными (они могут быть подписаны, но это не обязательно).

Чтобы ограничить срок действия URL (полезно для подписанных URL), можно указать опцию `vu` со значением Unix-метки времени. Например, следующий подписанный URL будет действителен только до 2020-01-01:

```
http://localhost:8080/vu1577836800,sjNcVf6LxzKEvR6Owgg3zhEMN7xbWxlpf-eyYbRfFK4A=/https://example.com/image
```

[signed codercat URL]: http://localhost:8080/500,sXyMwWKIC5JPCtlYOQ2f4yMBTqpjtUsfI67Sp7huXIYY=/https://octodex.github.com/images/codercat.jpg
[signature option]: https://pkg.go.dev/willnorris.com/go/imageproxy#hdr-Signature-ParseOptions

### Базовый URL по умолчанию

Обычно удалённые изображения задаются абсолютными URL. Однако, если вы часто проксируете изображения из одного источника, можно задать базовый URL и указывать удалённые изображения относительными путями. Попробуйте:

```sh
imageproxy -baseURL https://octodex.github.com/
```

Затем загрузите codercat, указав путь относительно базового URL:
<http://localhost:8080/500/images/codercat.jpg>. Заметьте, это не скрывает реальный источник изображений — легко узнать базовый URL. Даже при заданном базовом URL вы всегда можете передать абсолютный URL изображения.

### Несколько префиксов хранилищ

Если нужно, чтобы один экземпляр imageproxy обслуживал несколько исходных хранилищ, настройте флаг `storages` с JSON-объектом, который отображает первый сегмент пути на базовый URL:

```sh
imageproxy -storages '{"tms":"https://tms-storage.example.com/","hr":"https://hr-storage.example.com/"}'
```

Ту же настройку можно передать через переменную окружения `IMAGEPROXY_STORAGES`.

После этого запросы вида:

```text
http://localhost:8080/tms/path/to/image.jpg?x=300
http://localhost:8080/hr/avatars/user.jpg?x=300&y=200&fit=true
```

будут разрешаться относительно настроенного URL хранилища. Query-параметры
будут преобразованы во внутренние опции imageproxy и применены к полученному
из origin изображению.

Если нужно разрешать файлы в поддиректории, включите завершающий слеш в базовом URL, например `https://storage.example.com/bucket/`.

### Масштабирование больше оригинала

По умолчанию imageproxy не увеличивает изображение больше его оригинального размера. Чтобы разрешить увеличение, используйте флаг `-scaleUp`:

```sh
imageproxy -scaleUp true
```

### Поддержка WebP и TIFF

imageproxy может проксировать удалённые webp-изображения, но при трансформации они будут возвращены в формате jpeg или png (потому что golang-библиотека webp поддерживает только декодирование). Если формат явно не указан, по умолчанию будет использоваться jpeg. Если трансформация не требуется (например, вы используете imageproxy только как SSL-прокси), то оригинальное webp-изображение будет передано без конверсии.

Поскольку немногие браузеры поддерживают tiff, при трансформации tiff будет конвертироваться в jpeg по умолчанию. Чтобы принудительно получить tiff, передайте опцию `tiff`. Как и в случае с webp, если трансформация не выполняется, оригинальный tiff будет передан без изменений.

Запустите `imageproxy -help`, чтобы увидеть полный список флагов.

### Переменные окружения

Все флаги имеют эквивалент в переменных окружения вида `IMAGEPROXY_$NAME`. Например, кэш на диске можно настроить так:

```sh
IMAGEPROXY_CACHE="/tmp/imageproxy" imageproxy
```

Несколько префиксов хранилищ можно задать JSON-объектом:

```sh
IMAGEPROXY_STORAGES='{"tms":"https://tms-storage.example.com/","hr":"https://hr-storage.example.com/"}' imageproxy
```

## Развёртывание

В большинстве случаев достаточно стандартной процедуры сборки и развёртывания Go-приложения. Например:

- `go build willnorris.com/go/imageproxy/cmd/imageproxy`
- скопировать бинарник в `/usr/local/bin`
- скопировать [`etc/imageproxy.service`](etc/imageproxy.service) в `/lib/systemd/system` и включить через `systemctl`.

Ниже представлены инструкции, присланные сообществом, для запуска на других платформах.

### Heroku

Проще всего вынести зависимости в vendor с помощью `Godep` и задеплоить на Heroku. Посмотрите этот [репозиторий](https://github.com/oreillymedia/prototype-imageproxy/tree/heroku) (ветка `heroku`).

### AWS Elastic Beanstalk

[O’Reilly Media](https://github.com/oreillymedia) подготовили репозиторий с тем, что нужно для деплоя в Elastic Beanstalk. Следуйте инструкциям в их README.

### Docker

Docker-образ доступен как `ghcr.io/willnorris/imageproxy`.

Запустить контейнер можно так:

```sh
docker run -p 8080:8080 ghcr.io/willnorris/imageproxy -addr 0.0.0.0:8080
```

Или в вашем Dockerfile:

```Dockerfile
ENTRYPOINT ["/app/imageproxy", "-addr 0.0.0.0:8080"]
```

Если вы запускаете imageproxy в контейнере с примонтированным дисковым кэшем, убедитесь, что процесс внутри контейнера запущен от пользователя с правом записи в примонтированную директорию. Подробнее см. обсуждение в [#198](https://github.com/willnorris/imageproxy/issues/198).

Учтите, что все параметры можно задать через переменные окружения, что удобно для контейнеризации.

### Caddy

Можно проксировать запросы к imageproxy в конфигурации Caddy с помощью директивы `reverse_proxy`:

```Caddyfile
@imageproxy path /api/imageproxy/*
handle @imageproxy {
  uri replace /api/imageproxy/ /
  reverse_proxy http://localhost:4593
}
```

Также можно встроить экземпляр imageproxy в Caddy через модуль [caddy/], см. каталог `caddy/`. Для этого потребуется кастомная сборка Caddy с включённым модулем imageproxy (см. пример) и конфигурация в Caddyfile:

```Caddyfile
@imageproxy path /api/imageproxy/*
handle @imageproxy {
  uri replace /api/imageproxy/ /

  imageproxy {
    cache /data/imageproxy-cache
    default_base_url {$IMAGEPROXY_BASEURL}
    allow_hosts {$IMAGEPROXY_ALLOWHOSTS}
    signature_key {$IMAGEPROXY_SIGNATUREKEY}
  }
}
```

### nginx

Используйте `proxy_pass`, чтобы направить запросы в ваш экземпляр imageproxy. Например, чтобы обслуживать imageproxy по пути `/api/imageproxy/`, настройте:

```nginx
location /api/imageproxy/ {
  proxy_pass http://localhost:4593/;
}
```

В зависимости от других директив в вашей конфигурации, возможно, потребуется изменить приоритет, используя:

```nginx
location ^~ /api/imageproxy/ {
  proxy_pass http://localhost:4593/;
}
```

## Клиенты

- [Hugo partial](https://github.com/willnorris/willnorris.com/blob/main/layouts/partials/imageproxy-url.html) (использую вместе с `{{<img>}}` shortcode)
- [Ruby-клиент](https://github.com/azolf/imageproxy_ruby)

## Лицензия

imageproxy защищён правами своих авторов. Моя личная работа над imageproxy до 2020 года (которая составляет большую часть кода) — это работа, выполненная при поддержке Google, моего работодателя в то время. Проект распространяется под лицензией Apache 2.0 (см. ./LICENSE).
