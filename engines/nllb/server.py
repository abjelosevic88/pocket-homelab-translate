"""Pocket Translate's NLLB engine: NLLB-200 on the CPU behind the LibreTranslate API.

GET  /languages   [{"code", "name", "targets"}]
POST /translate   {"q": str | [str], "source": code | "auto", "target": code}
                  -> {"translatedText": str | [str], "detectedLanguage"?: {...}}
POST /detect      {"q": str} -> [{"language", "confidence"}]
GET  /health      200 when the model is loaded, 503 (with progress) before

Requests may be JSON or form-encoded, like LibreTranslate's. Set API_KEY to
require "api_key". Nothing is kept: text in, text out.
"""

import json
import os
import re
import sys
import threading
import time
import urllib.parse
import urllib.request
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

# ISO 639-1 (LibreTranslate's codes; "zt" = Chinese, traditional) -> FLORES-200.
CODES = {
    "af": ("afr_Latn", "Afrikaans"), "am": ("amh_Ethi", "Amharic"), "ar": ("arb_Arab", "Arabic"),
    "az": ("azj_Latn", "Azerbaijani"), "be": ("bel_Cyrl", "Belarusian"), "bg": ("bul_Cyrl", "Bulgarian"),
    "bn": ("ben_Beng", "Bengali"), "bs": ("bos_Latn", "Bosnian"), "ca": ("cat_Latn", "Catalan"),
    "cs": ("ces_Latn", "Czech"), "cy": ("cym_Latn", "Welsh"), "da": ("dan_Latn", "Danish"),
    "de": ("deu_Latn", "German"), "el": ("ell_Grek", "Greek"), "en": ("eng_Latn", "English"),
    "eo": ("epo_Latn", "Esperanto"), "es": ("spa_Latn", "Spanish"), "et": ("est_Latn", "Estonian"),
    "eu": ("eus_Latn", "Basque"), "fa": ("pes_Arab", "Persian"), "fi": ("fin_Latn", "Finnish"),
    "fr": ("fra_Latn", "French"), "ga": ("gle_Latn", "Irish"), "gl": ("glg_Latn", "Galician"),
    "gu": ("guj_Gujr", "Gujarati"), "ha": ("hau_Latn", "Hausa"), "he": ("heb_Hebr", "Hebrew"),
    "hi": ("hin_Deva", "Hindi"), "hr": ("hrv_Latn", "Croatian"), "hu": ("hun_Latn", "Hungarian"),
    "hy": ("hye_Armn", "Armenian"), "id": ("ind_Latn", "Indonesian"), "ig": ("ibo_Latn", "Igbo"),
    "is": ("isl_Latn", "Icelandic"), "it": ("ita_Latn", "Italian"), "ja": ("jpn_Jpan", "Japanese"),
    "ka": ("kat_Geor", "Georgian"), "kk": ("kaz_Cyrl", "Kazakh"), "km": ("khm_Khmr", "Khmer"),
    "kn": ("kan_Knda", "Kannada"), "ko": ("kor_Hang", "Korean"), "ky": ("kir_Cyrl", "Kyrgyz"),
    "lb": ("ltz_Latn", "Luxembourgish"), "lo": ("lao_Laoo", "Lao"), "lt": ("lit_Latn", "Lithuanian"),
    "lv": ("lvs_Latn", "Latvian"), "mk": ("mkd_Cyrl", "Macedonian"), "ml": ("mal_Mlym", "Malayalam"),
    "mn": ("khk_Cyrl", "Mongolian"), "mr": ("mar_Deva", "Marathi"), "ms": ("zsm_Latn", "Malay"),
    "mt": ("mlt_Latn", "Maltese"), "my": ("mya_Mymr", "Burmese"), "nb": ("nob_Latn", "Norwegian"),
    "ne": ("npi_Deva", "Nepali"), "nl": ("nld_Latn", "Dutch"), "pa": ("pan_Guru", "Punjabi"),
    "pl": ("pol_Latn", "Polish"), "ps": ("pbt_Arab", "Pashto"), "pt": ("por_Latn", "Portuguese"),
    "ro": ("ron_Latn", "Romanian"), "ru": ("rus_Cyrl", "Russian"), "si": ("sin_Sinh", "Sinhala"),
    "sk": ("slk_Latn", "Slovak"), "sl": ("slv_Latn", "Slovenian"), "so": ("som_Latn", "Somali"),
    "sq": ("als_Latn", "Albanian"), "sr": ("srp_Cyrl", "Serbian"), "sv": ("swe_Latn", "Swedish"),
    "sw": ("swh_Latn", "Swahili"), "ta": ("tam_Taml", "Tamil"), "te": ("tel_Telu", "Telugu"),
    "tg": ("tgk_Cyrl", "Tajik"), "th": ("tha_Thai", "Thai"), "tl": ("tgl_Latn", "Tagalog"),
    "tr": ("tur_Latn", "Turkish"), "uk": ("ukr_Cyrl", "Ukrainian"), "ur": ("urd_Arab", "Urdu"),
    "uz": ("uzn_Latn", "Uzbek"), "vi": ("vie_Latn", "Vietnamese"), "xh": ("xho_Latn", "Xhosa"),
    "yo": ("yor_Latn", "Yoruba"), "zh": ("zho_Hans", "Chinese"), "zt": ("zho_Hant", "Chinese (traditional)"),
    "zu": ("zul_Latn", "Zulu"),
}

FILES = ["config.json", "model.bin", "sentencepiece.bpe.model", "shared_vocabulary.txt"]
REPO = os.environ.get("MODEL_REPO", "JustFrederik/nllb-200-distilled-1.3B-ct2-int8")
MODEL_DIR = os.path.join(os.environ.get("MODEL_DIR", "/models"), REPO.replace("/", "--"))
API_KEY = os.environ.get("API_KEY", "")
THREADS = int(os.environ.get("THREADS", "4"))
BEAM = int(os.environ.get("BEAM", "2"))

state = {"ready": False, "detail": "starting", "error": ""}
translator = None
sp = None
detector = None
lock = threading.Lock()


def log(*a):
    print(time.strftime("%H:%M:%S"), *a, file=sys.stderr, flush=True)


def download():
    os.makedirs(MODEL_DIR, exist_ok=True)
    for name in FILES:
        dst = os.path.join(MODEL_DIR, name)
        if os.path.exists(dst):
            continue
        url = f"https://huggingface.co/{REPO}/resolve/main/{name}"
        tmp = dst + ".part"
        log("downloading", url)
        with urllib.request.urlopen(url, timeout=60) as r, open(tmp, "wb") as f:
            total = int(r.headers.get("Content-Length") or 0)
            got, last = 0, 0.0
            while True:
                b = r.read(1 << 20)
                if not b:
                    break
                f.write(b)
                got += len(b)
                if time.time() - last > 2:
                    last = time.time()
                    pct = f"{got * 100 // total}%" if total else f"{got >> 20} MB"
                    state["detail"] = f"downloading the model: {name} {pct}"
        os.replace(tmp, dst)


def load():
    global translator, sp, detector
    try:
        download()
        state["detail"] = "loading the model"
        import ctranslate2
        import sentencepiece

        translator = ctranslate2.Translator(MODEL_DIR, device="cpu", compute_type="int8", inter_threads=1, intra_threads=THREADS)
        sp = sentencepiece.SentencePieceProcessor(model_file=os.path.join(MODEL_DIR, "sentencepiece.bpe.model"))
        state["detail"] = "loading language detection"
        from lingua import Language, LanguageDetectorBuilder

        langs = [l for l in Language.all() if l.iso_code_639_1.name.lower() in CODES]
        detector = LanguageDetectorBuilder.from_languages(*langs).with_preloaded_language_models().build()
        state.update(ready=True, detail="ready")
        log("ready:", REPO)
    except Exception as e:  # noqa: BLE001 - reported on /health
        state.update(error=str(e), detail="failed: " + str(e))
        log("load failed:", e)


SENT = re.compile(r"(?<=[.!?…。！？])\s+")


def segments(text):
    """Splits text into (sentence, separator-after) pairs, keeping the layout."""
    out = []
    for line in re.split(r"(\n)", text):
        if line == "\n":
            if out:
                out[-1] = (out[-1][0], out[-1][1] + "\n")
            else:
                out.append(("", "\n"))
            continue
        lead = line[: len(line) - len(line.lstrip())]
        if lead:
            out.append(("", lead))
        parts = SENT.split(line.strip())
        for i, p in enumerate(parts):
            if p:
                out.append((p, " " if i < len(parts) - 1 else ""))
    return out


def chunks(tokens, n=200):
    return [tokens[i : i + n] for i in range(0, len(tokens), n)] or [[]]


def translate_texts(texts, src, tgt):
    s_code, t_code = CODES[src][0], CODES[tgt][0]
    plan = []  # per text: list of (piece index range, separator)
    sources = []
    for text in texts:
        segs = []
        for sent, sep in segments(text):
            if not sent.strip():
                segs.append((None, sent + sep))
                continue
            first = len(sources)
            for c in chunks(sp.encode(sent, out_type=str)):
                sources.append([s_code] + c + ["</s>"])
            segs.append(((first, len(sources)), sep))
        plan.append(segs)
    outs = []
    with lock:
        for i in range(0, len(sources), 24):
            batch = sources[i : i + 24]
            res = translator.translate_batch(batch, target_prefix=[[t_code]] * len(batch), beam_size=BEAM, max_decoding_length=400, max_batch_size=24)
            for r in res:
                toks = r.hypotheses[0]
                if toks and toks[0] == t_code:
                    toks = toks[1:]
                outs.append(sp.decode(toks))
    result = []
    for segs in plan:
        b = []
        for rng, sep in segs:
            if rng is None:
                b.append(sep)
            else:
                b.append(" ".join(outs[rng[0] : rng[1]]) + sep)
        result.append("".join(b))
    return result


def detect(text):
    if not text.strip():
        return []
    vals = detector.compute_language_confidence_values(text[:2000])
    out = []
    for v in vals[:3]:
        code = v.language.iso_code_639_1.name.lower()
        out.append({"language": code, "confidence": round(v.value * 100, 1)})
    return out


class Handler(BaseHTTPRequestHandler):
    server_version = "pocket-translate-nllb"

    def log_message(self, fmt, *args):
        pass

    def send(self, code, body):
        b = json.dumps(body, ensure_ascii=False).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Content-Length", str(len(b)))
        self.end_headers()
        self.wfile.write(b)

    def body(self):
        n = int(self.headers.get("Content-Length") or 0)
        raw = self.rfile.read(n) if n else b""
        ct = self.headers.get("Content-Type", "")
        if "json" in ct:
            return json.loads(raw or b"{}")
        q = urllib.parse.parse_qs(raw.decode(), keep_blank_values=True)
        return {k: (v if k == "q" and len(v) > 1 else v[0]) for k, v in q.items()}

    def ready(self):
        if not state["ready"]:
            self.send(503, {"error": "the NLLB engine is not ready: " + state["detail"]})
            return False
        return True

    def keyed(self, data):
        if API_KEY and data.get("api_key") != API_KEY:
            self.send(403, {"error": "Invalid API key"})
            return False
        return True

    def do_GET(self):
        path = self.path.split("?")[0]
        if path == "/health":
            self.send(200 if state["ready"] else 503, {"status": "ok" if state["ready"] else "loading", "detail": state["detail"], "model": REPO})
            return
        if path == "/languages":
            codes = sorted(CODES)
            self.send(200, [{"code": c, "name": CODES[c][1], "targets": [t for t in codes if t != c]} for c in codes])
            return
        self.send(404, {"error": "not found"})

    def do_POST(self):
        path = self.path.split("?")[0]
        try:
            data = self.body()
        except Exception:  # noqa: BLE001
            self.send(400, {"error": "invalid request"})
            return
        if not self.keyed(data) or not self.ready():
            return
        if path == "/detect":
            self.send(200, detect(str(data.get("q") or "")))
            return
        if path != "/translate":
            self.send(404, {"error": "not found"})
            return
        q = data.get("q")
        single = not isinstance(q, list)
        texts = [str(q or "")] if single else [str(x) for x in q]
        src, tgt = str(data.get("source") or "auto").lower(), str(data.get("target") or "").lower()
        if tgt not in CODES:
            self.send(400, {"error": f"{tgt} is not supported"})
            return
        detected = None
        if src == "auto":
            d = detect(" ".join(texts))
            src = d[0]["language"] if d else "en"
            detected = d[0] if d else {"language": "en", "confidence": 0}
        if src not in CODES:
            self.send(400, {"error": f"{src} is not supported"})
            return
        try:
            out = texts if src == tgt else translate_texts(texts, src, tgt)
        except Exception as e:  # noqa: BLE001
            log("translate failed:", e)
            self.send(500, {"error": "translation failed: " + str(e)})
            return
        res = {"translatedText": out[0] if single else out}
        if detected:
            res["detectedLanguage"] = detected
        self.send(200, res)


if __name__ == "__main__":
    threading.Thread(target=load, daemon=True).start()
    port = int(os.environ.get("PORT", "5000"))
    log("listening on", port)
    ThreadingHTTPServer(("0.0.0.0", port), Handler).serve_forever()
