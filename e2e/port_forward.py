"""Helpers for managing kubectl port-forward sessions."""
from __future__ import annotations

import subprocess
import time
from pathlib import Path
from typing import Optional


def start_port_forward(namespace: str, service_name: str, port: int, log_file: Path) -> subprocess.Popen:
    if port <= 0:
        port = 18080
    port_arg = f"{port}:8080"
    command = [
        "kubectl",
        "-n",
        namespace,
        "port-forward",
        f"svc/{service_name}",
        port_arg,
    ]
    with log_file.open("w", encoding="utf-8") as log:
        process = subprocess.Popen(command, stdout=log, stderr=log)
    return process


def stop_port_forward(process: Optional[subprocess.Popen]) -> None:
    if process is None:
        return
    if process.poll() is not None:
        return
    process.terminate()
    try:
        process.wait(timeout=5)
    except subprocess.TimeoutExpired:
        process.kill()


def wait_for_registry(base_url: str, timeout: int = 30) -> bool:
    import urllib.request
    deadline = time.time() + timeout
    while time.time() < deadline:
        try:
            with urllib.request.urlopen(f"{base_url}/v1/tools", timeout=5):
                return True
        except Exception:
            time.sleep(1)
    return False
