FROM python:3.15.0rc3-slim-bookworm

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1

WORKDIR /app

RUN apt-get update \
  && apt-get install -y --no-install-recommends curl \
  && rm -rf /var/lib/apt/lists/*

COPY requirements.lock .
RUN --mount=type=cache,target=/root/.cache/pip \
  pip install --require-hashes -r requirements.lock

COPY . .

RUN groupadd --system --gid 10001 alphalens \
  && useradd --system --uid 10001 --gid alphalens --home-dir /app alphalens \
  && mkdir -p /app/data \
  && chown -R alphalens:alphalens /app

USER 10001:10001

EXPOSE 8765

CMD ["uvicorn", "main:app", "--host", "0.0.0.0", "--port", "8765"]
