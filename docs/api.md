# API

Everything under `/api/v1`, JSON, with a session cookie or `Authorization: Bearer ptl_…` (Settings → API & shortcuts). Every row belongs to the person the token is theirs; other people's ids answer 404.

## Translating

| | |
|---|---|
| `POST /translate` | `{text, from?, to?, engine?: auto\|ai\|offline, tone?: formal\|casual, alternatives?, glossary?: true, save?}` → `{text, detected, engine, provider, alternatives, terms, note, translation?}`. `texts: [...]` (up to 200) translates several at once → `{texts}` (not kept). |
| `POST /detect` | `{text}` → `{language, name, by}` |
| `POST /explain` | `{text, from, to}` → `{meaning, literal, usage, register, examples}` (AI) |
| `GET /languages` | every language with `argos`, `nllb` (offline engines that know it) and `read` (photos) |
| `GET /engines` | which engines work for you, and why not |
| `POST /ocr` | multipart `file` (a photo), `from?`, `langs?` → `{text}` |
| `POST /speech` | multipart `file` (a recording), `language?` → `{text, language}` |

Language codes are ISO 639-1, with `sr-Latn` / `sr-Cyrl` for Serbian's two alphabets, `zt` for traditional Chinese and `pb` for Brazilian Portuguese.

## History, phrasebook, glossary

`GET /translations?q=&starred=1&collection=&kind=&lang=&limit=&offset=`, `POST /translations` (a phrase; translated when `result_text` is empty), `GET|PUT|DELETE /translations/{id}`, `DELETE /translations` (history; `?all=1` with the phrasebook), `POST /translations/{id}/learn` (Pocket Learn), `POST /translations/{id}/note` (Pocket Notes), `GET /collections`, `POST /collections/rename`.

`GET|POST /glossary`, `PUT|DELETE /glossary/{id}`, `POST /glossary/import?from=&to=` (CSV/TSV body), `GET /glossary.csv`.

## Documents

`POST /documents` (multipart `file`, `from`, `to`, `engine`) or `POST /documents/from` (`{app: files|documents, ref}`), then `GET /documents/{id}` until `status` is `done`, and `GET /documents/{id}/result`. `POST /documents/{id}/save` (`{app: files|documents}`), `POST /documents/{id}/retry`, `DELETE /documents/{id}`.

## Conversations

`GET|POST /conversations` (`{lang_a, lang_b, title?}`), `GET|PUT|DELETE /conversations/{id}`, `POST /conversations/{id}/turns` (multipart `file` + `side`, or `{side, text}`), `POST /turns/{id}/retry`, `DELETE /turns/{id}`, `GET /conversations/{id}/transcript.txt`, `POST /conversations/{id}/note`.

## For the rest of the box

`GET /summary`, `GET /search?q=`, `GET /day/{YYYY-MM-DD}`; `GET /export.json`, `GET /export.csv?starred=1`, `POST /import` (an export, Google Translate's saved phrases or a CSV).

## The LibreTranslate API

`POST /translate` (`q`, `source`, `target`, `format: text|html`, `alternatives`, `api_key`), `POST /detect`, `GET /languages` — at the root, like LibreTranslate. The `api_key` is a Pocket Translate token. Uses the person's engine choice and glossary; nothing is kept in the history.
