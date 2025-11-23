FROM python:3.11-slim
RUN apt-get update && apt-get install -y build-essential && apt-get clean && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY pyproject.toml README.md /app/
COPY src /app/src
RUN pip install --upgrade pip && pip install -e .
ENV HISTORY_DB=/cache/history.db
CMD ["uvicorn", "s390x_wheel_refinery.webapp:app", "--host", "0.0.0.0", "--port", "8000"]
