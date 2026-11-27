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

`GET /summary`, `GET /search?q=`, `GET /day/{YYYY-MM-DD}`; `GET /export.json`, `GET /export.csv?starred=1`, `POST /import` (an export, Google Translate's saved phrases or a CSV; what is already here is skipped), `GET /pocket/portable` (what Pocket Move can copy out and bring in, for `move:export` / `move:import` links).

### Pocket Search and Pocket Hub

The routes every Pocket app answers in the same shape (`docs/capabilities.md` in pocket-homelab-core). They read what is kept here and never translate anything; a Core link token opens each one with its own scope only.

- `GET /pocket/search?q=&limit=` (`pocket:search`): saved phrases (kind `phrase`, id `phrase:12`), then glossary terms (kind `term`, id `term:4`), then the rest of the history (kind `translation`, id `translation:13`), found as `GET /search` finds them: phrases and history by the text, its translation, the note or the collection, newest first; terms by the term, its translation or the note. The title is the text or the term, the line under it the languages and the translation (and a term's note); the link opens the Phrasebook or History page searching for it, or the Glossary page.
- `GET /pocket/widget` (`pocket:widget`): how many phrases the phrasebook holds and the newest five with their translation; nothing when the phrasebook is empty.

There is no `/pocket/day` (Pocket Day reads `GET /day/{date}` through `phrasebook:read`) and no `/pocket/capture`: a phrase saved without its translation is translated on the spot, which takes too long for a quick line.

## The LibreTranslate API

`POST /translate` (`q`, `source`, `target`, `format: text|html`, `alternatives`, `api_key`), `POST /detect`, `GET /languages` — at the root, like LibreTranslate. The `api_key` is a Pocket Translate token. Uses the person's engine choice and glossary; nothing is kept in the history.
