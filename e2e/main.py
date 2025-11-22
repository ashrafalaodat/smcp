"""Entry point for running registry E2E scenarios."""
from __future__ import annotations

import argparse
import logging
import os
import shutil
import signal
import subprocess
import sys
import tempfile
import uuid
from pathlib import Path
from typing import List, Optional

from .client import RegistryClient
from .port_forward import start_port_forward, stop_port_forward, wait_for_registry
from .scenarios import ScenarioConfig, run_registry_semantic_search

LOGGER = logging.getLogger(__name__)


def parse_args(argv: Optional[List[str]] = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Run MCP registry end-to-end tests")
    parser.add_argument("--base-url", default=os.getenv("BASE_URL"), help="Registry base URL; skips port-forwarding when provided")
    parser.add_argument("--namespace", default=os.getenv("NAMESPACE", "default"))
    parser.add_argument("--service-name", default=os.getenv("SERVICE_NAME", "mcp-registry"))
    parser.add_argument("--forward-port", type=int, default=int(os.getenv("FORWARD_PORT", "18080")),
                        help="Local port to forward to the registry (default 18080)")
    parser.add_argument("--log-level", default=os.getenv("LOG_LEVEL", "INFO"))
    return parser.parse_args(argv)


def ensure_commands(*commands: str) -> None:
    for command in commands:
        if shutil.which(command) is None:
            raise RuntimeError(f"Required command not found: {command}")



def main(argv: Optional[List[str]] = None) -> int:
    args = parse_args(argv)

    logging.basicConfig(level=getattr(logging, args.log_level.upper(), logging.INFO), format="%(message)s")

    tmp_dir = Path(tempfile.mkdtemp(prefix="registry-e2e-"))
    port_forward_process = None
    base_url_value: Optional[str] = args.base_url
    client_ref: List[Optional[RegistryClient]] = [None]

    def cleanup(signum: Optional[int] = None, frame: Optional[object] = None) -> None:  # noqa: ARG001
        LOGGER.debug("Cleaning up resources")
        # Nothing to clean up on the server side; registry API is idempotent per owner/name.
        stop_port_forward(port_forward_process)
        try:
            shutil.rmtree(tmp_dir, ignore_errors=True)
        except Exception as exc:  # noqa: BLE001
            LOGGER.debug("Failed to cleanup temp dir: %s", exc)
        if signum is not None:
            sys.exit(1)

    signal.signal(signal.SIGINT, cleanup)
    signal.signal(signal.SIGTERM, cleanup)

    try:
        if base_url_value is None:
            ensure_commands("kubectl")
            log_file = tmp_dir / "port-forward.log"
            LOGGER.info("Starting port-forward to svc/%s", args.service_name)
            port_forward_process = start_port_forward(args.namespace, args.service_name, args.forward_port, log_file)
            base_url_value = f"http://127.0.0.1:{args.forward_port}"
            if not wait_for_registry(base_url_value):
                raise RuntimeError(
                    "timed out waiting for registry readiness. See log: " + str(log_file)
                )
        LOGGER.info("Using registry at %s", base_url_value)

        client = RegistryClient(base_url_value)
        client_ref[0] = client

        suffix = uuid.uuid4().hex[:8]
        scenario_config = ScenarioConfig(
            tool_owner="team-search",
            primary_tool_name=f"vector-summarize-{suffix}",
            primary_description="Summarises documents into short bullet lists",
            summarizer_variants=(
                "kb-digest::Summarises knowledge base articles into compact bullet lists",
                "chat-brief::Summarises customer conversations into concise action items",
            ),
            other_tools=(
                "image-classifier::Classifies product photos into catalog categories",
                "intent-detector::Identifies customer intent from support tickets",
                "sentiment-analyzer::Scores sentiment across user feedback",
                "translation-service::Translates marketing copy between languages",
                "entity-extractor::Extracts named entities from compliance reports",
                "faq-matcher::Matches user questions to knowledge-base answers",
                "contract-review::Highlights risky clauses in vendor contracts",
            ),
            suffix=suffix,
        )

        run_registry_semantic_search(client, scenario_config)

        LOGGER.info("E2E scenario completed successfully")
        return 0
    except Exception as exc:  # noqa: BLE001
        LOGGER.error("E2E scenario failed: %s", exc)
        return 1
    finally:
        cleanup()


if __name__ == "__main__":
    sys.exit(main())
