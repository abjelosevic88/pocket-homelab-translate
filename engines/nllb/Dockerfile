# Pocket Translate's second offline engine: NLLB-200 (Meta, 200 languages,
# Serbian, Croatian and Bosnian among them) on the CPU with CTranslate2.
# It answers the LibreTranslate API (/languages, /translate, /detect), so
# Pocket Translate talks to it like to LibreTranslate. The model (about
# 1.4 GB for 1.3B int8) is downloaded on the first start into /models.
FROM python:3.12-slim
RUN useradd -u 1000 -m app && mkdir /models && chown app /models
WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY server.py .
USER app
ENV MODEL_REPO=JustFrederik/nllb-200-distilled-1.3B-ct2-int8 MODEL_DIR=/models PORT=5000 THREADS=4
EXPOSE 5000
HEALTHCHECK --interval=30s --timeout=5s --start-period=30m CMD python -c "import urllib.request;urllib.request.urlopen('http://127.0.0.1:5000/health',timeout=4)"
CMD ["python", "-u", "server.py"]
