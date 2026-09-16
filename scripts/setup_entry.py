from pathlib import Path
import argparse
import sys

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

from aetheris.setup_app import run_setup_gui
from aetheris.version import __version__


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="Aetheris Windows Setup")
    parser.add_argument("--version", action="version", version=__version__)
    parser.parse_args()
    run_setup_gui()
