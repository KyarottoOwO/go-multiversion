"""Explicit .30 extraction runner; use a disposable BDS in a network namespace."""
import argparse
import hashlib
import json
import os
import pathlib
import shutil
import socket
import subprocess
import time

parser = argparse.ArgumentParser()
parser.add_argument("mode", choices=["vanilla", "palette"])
parser.add_argument("--server", type=pathlib.Path, required=True)
parser.add_argument("--output", type=pathlib.Path, required=True)
parser.add_argument("--capture", type=pathlib.Path)
parser.add_argument("--pack", type=pathlib.Path, default=pathlib.Path(__file__).parent / "palette-pack")
args = parser.parse_args()
server, output = args.server.resolve(), args.output.resolve()
binary = server / "bedrock_server"
expected = "2c76ff3184906259f3964e5bf2ab9db414f1570b8179a3561637a134a7de72e7"
if hashlib.sha256(binary.read_bytes()).hexdigest() != expected:
    raise SystemExit("Refusing a different BDS binary")
if [name for _, name in socket.if_nameindex()] != ["lo"]:
    raise SystemExit("Run through unshare -Urn; only loopback is allowed")
subprocess.run(["ip", "link", "set", "lo", "up"], check=True)
worlds = server / "worlds"
world = worlds / "native2225-extraction"
if worlds.exists() and any(p.name != world.name for p in worlds.iterdir()):
    raise SystemExit("Refusing a server directory containing other worlds")
if args.mode == "vanilla" and world.exists():
    raise SystemExit("Vanilla acquisition requires a new extraction world")
if args.mode == "palette":
    if not world.exists():
        raise SystemExit("Capture vanilla data before adding the extraction pack")
    shutil.copytree(args.pack, server / "behavior_packs/native2225_palette", dirs_exist_ok=True)
    (world / "world_behavior_packs.json").write_text(json.dumps([
        {"pack_id": "c69bb36f-6d41-4e5c-85e2-9dc356763b81", "version": [1, 0, 0]}
    ]))
overrides = {
    "server-name": "Native 2225 extraction", "gamemode": "creative", "difficulty": "peaceful",
    "allow-cheats": "true", "max-players": "1", "online-mode": "false", "allow-list": "false",
    "server-port": "19172", "server-portv6": "19173", "transport": "raknet",
    "enable-lan-visibility": "false", "view-distance": "5", "tick-distance": "4",
    "level-name": world.name, "level-seed": "1", "default-player-permission-level": "operator",
    "content-log-console-output-enabled": "true", "content-log-file-enabled": "true",
    "emit-server-telemetry": "false",
}
properties = server / "server.properties"
lines = properties.read_text().splitlines()
for key, value in overrides.items():
    lines = [line for line in lines if not line.startswith(key + "=")]
    lines.append(key + "=" + value)
properties.write_text("\n".join(lines) + "\n")
output.mkdir(parents=True, exist_ok=True)
(output / "settings.json").write_text(json.dumps(overrides, indent=2) + "\n")
log_path = output / (args.mode + ".log")
with log_path.open("w") as log:
    process = subprocess.Popen([str(binary)], cwd=server, stdin=subprocess.PIPE,
        stdout=log, stderr=subprocess.STDOUT, text=True,
        env=dict(os.environ, LD_LIBRARY_PATH=str(server)))
    try:
        for second in range(300):
            time.sleep(1)
            text = log_path.read_text(errors="replace")
            if process.poll() is not None:
                raise RuntimeError("BDS exited during extraction")
            if args.mode == "vanilla" and "Server started." in text:
                if args.capture is None:
                    raise RuntimeError("Vanilla mode requires --capture")
                subprocess.run([str(args.capture.resolve()), "-output", str(output / "capture")],
                    cwd=server, timeout=60, check=True)
                break
            if args.mode == "palette" and "NATIVE2225_DONE " in text:
                break
            if "[Scripting]" in text and any(e in text for e in ["TypeError:", "ReferenceError:", "] Error:"]):
                raise RuntimeError("Extraction script failed; inspect the log")
            if second % 20 == 19:
                print("Extraction running", second + 1, "seconds", flush=True)
        else:
            raise RuntimeError("Extraction timed out")
    finally:
        if process.poll() is None:
            process.stdin.write("stop\n")
            process.stdin.flush()
            try:
                process.wait(timeout=20)
            except subprocess.TimeoutExpired:
                process.terminate()
                process.wait(timeout=5)
        print("Extraction BDS stopped", flush=True)
