#!/usr/bin/env python3
from __future__ import annotations

import logging
import runpy
import sys


logger = logging.getLogger("rebecca.binary")


def clear_app_imports() -> None:
    for module_name in list(sys.modules):
        if (
            module_name == "app"
            or module_name.startswith("app.")
            or module_name == "dashboard"
            or module_name.startswith("dashboard.")
        ):
            sys.modules.pop(module_name, None)


def main() -> None:
    logging.basicConfig(level=logging.INFO, format="%(asctime)s [%(levelname)s] %(message)s")
    clear_app_imports()
    runpy.run_module("main", run_name="__main__")


if __name__ == "__main__":
    main()
