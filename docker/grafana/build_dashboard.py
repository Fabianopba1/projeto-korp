#!/usr/bin/env python3
"""Gera o dashboard do Grafana para o http-server-projeto-korp.

Escrever o JSON via script (em vez de exportar da UI) mantem o arquivo
legivel, versionavel e comentavel.
"""
import json
from pathlib import Path

DS = {"type": "prometheus", "uid": "prometheus-korp"}
JOB = 'job="http-server-projeto-korp"'


def target(expr, legend="", ref="A", instant=False):
    t = {
        "datasource": DS,
        "editorMode": "code",
        "expr": expr,
        "legendFormat": legend or "__auto",
        "range": not instant,
        "instant": instant,
        "refId": ref,
    }
    return t


def stat(pid, title, expr, x, y, w, h, unit="none", decimals=None,
         mappings=None, thresholds=None, desc="", color_mode="value",
         graph_mode="none"):
    defaults = {
        "color": {"mode": "thresholds"},
        "mappings": mappings or [],
        "unit": unit,
        "thresholds": thresholds or {
            "mode": "absolute",
            "steps": [{"color": "text", "value": None}],
        },
    }
    if decimals is not None:
        defaults["decimals"] = decimals

    return {
        "id": pid,
        "type": "stat",
        "title": title,
        "description": desc,
        "datasource": DS,
        "gridPos": {"h": h, "w": w, "x": x, "y": y},
        "fieldConfig": {"defaults": defaults, "overrides": []},
        "options": {
            "colorMode": color_mode,
            "graphMode": graph_mode,
            "justifyMode": "auto",
            "orientation": "auto",
            "reduceOptions": {
                # lastNotNull, nao "lastNonNull": esse e o identificador do
                # redutor no Grafana. Qualquer outra string e ignorada
                # silenciosamente e o painel fica sem valor -- o sparkline
                # ainda desenha, porque usa a serie crua sem passar pelo
                # redutor, o que torna o sintoma confuso.
                "calcs": ["lastNotNull"],
                "fields": "",
                "values": False,
            },
            # "value" e nao "auto": com "auto" o Grafana desenha nome + valor,
            # e o nome gerado pelo legendFormat "__auto" e a serie inteira
            # com todos os labels ({__name__="up", instance=..., job=...}).
            # Esse nome consome o espaco do painel e o numero nao aparece.
            "textMode": "value",
            "wideLayout": True,
            "percentChangeColorMode": "standard",
            "showPercentChange": False,
        },
        # Consulta de INTERVALO, nao instant. Paineis stat com instant=true
        # nao renderizaram valor no Grafana 11; com range + lastNotNull o
        # comportamento e identico ao dos paineis de serie temporal, que
        # funcionam. Menos elegante, mas comprovadamente correto.
        "targets": [target(expr, instant=False)],
        "pluginVersion": "11.1.0",
    }


def timeseries(pid, title, targets, x, y, w, h, unit="none", desc="",
               fill=10, stack=False, minval=None, maxval=None,
               legend_calcs=None, draw_style="line"):
    custom = {
        "axisBorderShow": False,
        "axisCenteredZero": False,
        "axisColorMode": "text",
        "axisLabel": "",
        "axisPlacement": "auto",
        "barAlignment": 0,
        "drawStyle": draw_style,
        "fillOpacity": fill,
        "gradientMode": "opacity",
        "hideFrom": {"legend": False, "tooltip": False, "viz": False},
        "insertNulls": False,
        "lineInterpolation": "smooth",
        "lineWidth": 2,
        "pointSize": 5,
        "scaleDistribution": {"type": "linear"},
        "showPoints": "never",
        "spanNulls": False,
        "stacking": {
            "group": "A",
            "mode": "normal" if stack else "none",
        },
        "thresholdsStyle": {"mode": "off"},
    }

    defaults = {
        "color": {"mode": "palette-classic"},
        "custom": custom,
        "mappings": [],
        "unit": unit,
        "thresholds": {
            "mode": "absolute",
            "steps": [{"color": "green", "value": None}],
        },
    }
    if minval is not None:
        defaults["min"] = minval
    if maxval is not None:
        defaults["max"] = maxval

    return {
        "id": pid,
        "type": "timeseries",
        "title": title,
        "description": desc,
        "datasource": DS,
        "gridPos": {"h": h, "w": w, "x": x, "y": y},
        "fieldConfig": {"defaults": defaults, "overrides": []},
        "options": {
            "legend": {
                "calcs": legend_calcs or ["mean", "max", "lastNotNull"],
                "displayMode": "table",
                "placement": "bottom",
                "showLegend": True,
            },
            "tooltip": {"mode": "multi", "sort": "desc"},
        },
        "targets": targets,
        "pluginVersion": "11.1.0",
    }


def row(pid, title, y):
    return {
        "id": pid,
        "type": "row",
        "title": title,
        "collapsed": False,
        "gridPos": {"h": 1, "w": 24, "x": 0, "y": y},
        "panels": [],
    }


panels = []

# =========================================================================
# LINHA 1 - DISPONIBILIDADE (requisito obrigatorio do desafio)
# =========================================================================
panels.append(row(100, "Disponibilidade do Servico", 0))

panels.append(stat(
    1, "Status Atual", f'up{{{JOB}}}', 0, 1, 4, 5,
    desc=("A metrica `up` e gerada pelo proprio Prometheus a cada scrape: "
          "vale 1 se o alvo respondeu e 0 se falhou. E o sinal canonico de "
          "disponibilidade."),
    color_mode="background",
    mappings=[{
        "type": "value",
        "options": {
            "0": {"text": "FORA DO AR", "color": "red", "index": 0},
            "1": {"text": "NO AR", "color": "green", "index": 1},
        },
    }],
    thresholds={"mode": "absolute", "steps": [
        {"color": "red", "value": None},
        {"color": "green", "value": 1},
    ]},
))

panels.append(stat(
    2, "Disponibilidade (6h)",
    f'avg_over_time(up{{{JOB}}}[6h]) * 100',
    4, 1, 5, 5, unit="percent", decimals=3,
    desc=("Como `up` so vale 0 ou 1, a media no periodo equivale a fracao "
          "do tempo em que o servico esteve no ar."),
    thresholds={"mode": "absolute", "steps": [
        {"color": "red", "value": None},
        {"color": "orange", "value": 99},
        {"color": "green", "value": 99.9},
    ]},
))

panels.append(stat(
    3, "Taxa de Sucesso (5m)",
    (f'sum(rate(korp_http_requests_total{{status=~"2..|3.."}}[5m]))'
     f' / clamp_min(sum(rate(korp_http_requests_total[5m])), 0.0001) * 100'),
    9, 1, 5, 5, unit="percent", decimals=2,
    desc=("Disponibilidade sob a otica do cliente: proporcao de respostas "
          "bem-sucedidas. Um servico pode estar `up` e ainda assim retornar "
          "erro para todo mundo. clamp_min evita divisao por zero quando "
          "nao ha trafego."),
    thresholds={"mode": "absolute", "steps": [
        {"color": "red", "value": None},
        {"color": "orange", "value": 95},
        {"color": "green", "value": 99},
    ]},
))

panels.append(stat(
    4, "Uptime do Processo", "korp_uptime_seconds",
    14, 1, 5, 5, unit="s", decimals=0,
    desc=("Volta a zero a cada restart. Detecta crash loop mesmo quando o "
          "reinicio e rapido demais para o `up` registrar queda."),
    graph_mode="area",
    thresholds={"mode": "absolute", "steps": [
        {"color": "orange", "value": None},
        {"color": "green", "value": 300},
    ]},
))

panels.append(stat(
    5, "Versao em Execucao", "korp_build_info",
    19, 1, 5, 5,
    desc="Info metric: o valor e sempre 1, a informacao util esta nos labels.",
    color_mode="none",
))
# Info metric: o valor e sempre 1 e a informacao util esta no label.
# textMode "name" faz o painel exibir o NOME da serie, e o legendFormat
# define esse nome como o proprio label version.
panels[-1]["targets"][0]["legendFormat"] = "{{version}}"
panels[-1]["options"]["textMode"] = "name"

panels.append(timeseries(
    6, "Historico de Disponibilidade",
    [target(f'up{{{JOB}}}', "servico no ar")],
    0, 6, 24, 6, minval=0, maxval=1,
    desc="Cada vale neste grafico e uma janela de indisponibilidade.",
    fill=30, draw_style="line", legend_calcs=["mean", "min"],
))

# =========================================================================
# LINHA 2 - VOLUME DE REQUISICOES (requisito obrigatorio do desafio)
# =========================================================================
panels.append(row(200, "Volume de Requisicoes", 12))

panels.append(stat(
    7, "Requisicoes/s (agora)",
    "sum(rate(korp_http_requests_total[5m]))",
    0, 13, 4, 5, unit="reqps", decimals=2,
    desc="rate() calcula a taxa por segundo e trata reset de contador.",
    graph_mode="area",
    thresholds={"mode": "absolute", "steps": [{"color": "blue", "value": None}]},
))

panels.append(stat(
    8, "Total Acumulado",
    "sum(korp_http_requests_total)",
    4, 13, 4, 5, decimals=0,
    desc="Contador desde o ultimo start do processo.",
    thresholds={"mode": "absolute", "steps": [{"color": "purple", "value": None}]},
))

panels.append(stat(
    9, "Taxa de Erro 5xx (5m)",
    # "or vector(0)" garante um zero quando nenhum 5xx foi registrado.
    # Sem isso o numerador nao retorna serie alguma e o painel exibe
    # "No data" -- que e tecnicamente correto, mas parece defeito.
    ('(sum(rate(korp_http_requests_total{status=~"5.."}[5m])) or vector(0))'
     ' / clamp_min(sum(rate(korp_http_requests_total[5m])), 0.0001) * 100'),
    8, 13, 4, 5, unit="percent", decimals=2,
    desc="Percentual de respostas com erro do servidor.",
    thresholds={"mode": "absolute", "steps": [
        {"color": "green", "value": None},
        {"color": "orange", "value": 1},
        {"color": "red", "value": 5},
    ]},
))

panels.append(timeseries(
    10, "Requisicoes por Rota",
    [target('sum by (path) (rate(korp_http_requests_total[$__rate_interval]))',
            "{{path}}")],
    12, 13, 12, 10, unit="reqps",
    desc=("O label `path` usa o padrao da rota registrada, nao a URL crua. "
          "Isso impede que um cliente gere cardinalidade infinita batendo "
          "em /a, /b, /c... e derrube o Prometheus."),
    stack=True,
))

panels.append(timeseries(
    11, "Requisicoes por Status HTTP",
    [target('sum by (status) (rate(korp_http_requests_total[$__rate_interval]))',
            "HTTP {{status}}")],
    0, 18, 12, 5, unit="reqps", stack=True,
    desc="Separa 2xx, 4xx e 5xx para distinguir erro do cliente de erro do servidor.",
))

# =========================================================================
# LINHA 3 - LATENCIA E RECURSOS
# =========================================================================
panels.append(row(300, "Latencia e Recursos", 23))

panels.append(timeseries(
    12, "Latencia por Percentil",
    [
        target('histogram_quantile(0.50, sum by (le) '
               '(rate(korp_http_request_duration_seconds_bucket[$__rate_interval])))',
               "p50 (mediana)", "A"),
        target('histogram_quantile(0.95, sum by (le) '
               '(rate(korp_http_request_duration_seconds_bucket[$__rate_interval])))',
               "p95", "B"),
        target('histogram_quantile(0.99, sum by (le) '
               '(rate(korp_http_request_duration_seconds_bucket[$__rate_interval])))',
               "p99", "C"),
    ],
    0, 24, 12, 8, unit="s",
    desc=("Percentis reconstruidos a partir dos buckets do histograma. "
          "Media esconde outlier; p99 mostra a experiencia do usuario "
          "mais lento."),
    fill=0,
))

panels.append(timeseries(
    13, "Requisicoes Simultaneas",
    [target("korp_http_requests_in_flight", "em processamento")],
    12, 24, 12, 8,
    desc="Gauge de concorrencia. Crescimento sustentado indica gargalo.",
))

panels.append(timeseries(
    14, "Memoria (heap em uso)",
    [target('go_memstats_heap_inuse_bytes{' + JOB + '}', "heap em uso")],
    0, 32, 8, 7, unit="bytes",
    desc="Vem do coletor padrao do Go. Crescimento continuo sugere vazamento.",
))

panels.append(timeseries(
    15, "Goroutines",
    [target('go_goroutines{' + JOB + '}', "goroutines")],
    8, 32, 8, 7,
    desc="Contagem crescente sem estabilizar indica goroutine leak.",
))

panels.append(timeseries(
    16, "CPU do Processo",
    [target('rate(process_cpu_seconds_total{' + JOB + '}[$__rate_interval])',
            "cores em uso")],
    16, 32, 8, 7, unit="percentunit",
    desc="Fracao de um core consumida pelo processo.",
))

dashboard = {
    "annotations": {
        "list": [{
            "builtIn": 1,
            "datasource": {"type": "grafana", "uid": "-- Grafana --"},
            "enable": True,
            "hide": True,
            "iconColor": "rgba(0, 211, 255, 1)",
            "name": "Annotations & Alerts",
            "type": "dashboard",
        }]
    },
    "description": ("Observabilidade do servico http-server-projeto-korp: "
                    "disponibilidade, volume de requisicoes, latencia e recursos."),
    "editable": True,
    "fiscalYearStartMonth": 0,
    "graphTooltip": 1,  # crosshair compartilhado entre paineis
    "links": [],
    "panels": panels,
    "preload": False,
    "refresh": "10s",
    "schemaVersion": 39,
    "tags": ["projeto-korp", "golang", "desafio"],
    "templating": {"list": []},
    "time": {"from": "now-1h", "to": "now"},
    "timepicker": {},
    "timezone": "utc",
    "title": "HTTP Server Projeto Korp",
    "uid": "http-server-projeto-korp",
    "version": 1,
    "weekStart": "",
}

OUT = str(Path(__file__).parent / "provisioning" / "dashboards"
          / "http-server-projeto-korp-dashboard.json")

with open(OUT, "w", encoding="utf-8") as fh:
    json.dump(dashboard, fh, indent=2, ensure_ascii=False)
    fh.write("\n")

print(f"Dashboard gerado: {OUT}")
print(f"Total de paineis: {len([p for p in panels if p['type'] != 'row'])}")
print(f"Linhas (rows): {len([p for p in panels if p['type'] == 'row'])}")
