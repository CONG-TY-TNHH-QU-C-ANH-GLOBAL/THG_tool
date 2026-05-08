FROM python:3.12-slim

WORKDIR /app
COPY requirements.txt ./requirements.txt
RUN pip install --no-cache-dir -r requirements.txt
COPY brain.py planner_llm.py ./

ENV AGENT_BRAIN_HOST=0.0.0.0
ENV AGENT_BRAIN_PORT=8091

EXPOSE 8091
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD python -c "import urllib.request; urllib.request.urlopen('http://127.0.0.1:8091/healthz', timeout=2).read()" || exit 1

CMD ["python", "brain.py"]
