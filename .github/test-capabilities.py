#!/usr/bin/env python3
"""Exercise CoreDNS under real Linux credentials in a private network namespace."""
import argparse
import http.client
import ipaddress
import json
import os
from pathlib import Path
import shutil
import socket
import ssl
import struct
import subprocess
import tempfile
import time


def query(host, port, tcp):
    question = b"\x05probe\x07example\x00\x00\x01\x00\x01"
    packet = struct.pack("!6H", 1234, 0x100, 1, 0, 0, 0) + question
    family = socket.AF_INET6 if ":" in host else socket.AF_INET
    with socket.socket(family, socket.SOCK_STREAM if tcp else socket.SOCK_DGRAM) as sock:
        sock.settimeout(0.3)
        sock.connect((host, port))
        if tcp:
            sock.sendall(struct.pack("!H", len(packet)) + packet)
            prefix = b""
            while len(prefix) < 2:
                data = sock.recv(2 - len(prefix))
                if not data:
                    raise OSError("EOF before DNS frame")
                prefix += data
            size = struct.unpack("!H", prefix)[0]
            answer = b""
            while len(answer) < size:
                data = sock.recv(size - len(answer))
                if not data:
                    raise OSError("EOF during DNS frame")
                answer += data
        else:
            sock.send(packet)
            answer = sock.recv(4096)
    ident, flags, _, count, _, _ = struct.unpack("!6H", answer[:12])
    assert ident == 1234 and flags & 0x8000 and flags & 15 == 0 and count == 1, answer.hex()
    assert answer[-4:] == ipaddress.ip_address("192.0.2.1").packed, answer.hex()


def run_case(binary, directory, name, bounding, threshold, port=None, host="127.0.0.1", flag=None, success=True,
             no_new_privs=False, clear_permitted=False, uid=65532, reload_port=None, health_port=None,
             dnstap_port=None, dnstap_tls=False):
    subprocess.run(["sysctl", "-qw", f"net.ipv4.ip_unprivileged_port_start={threshold}"], check=True)
    cmd = ["setpriv", f"--reuid={uid}", f"--regid={uid}", "--clear-groups",
           f"--bounding-set={bounding}", "--inh-caps=-all", "--ambient-caps=-all"]
    if no_new_privs:
        cmd.append("--no-new-privs")
    if clear_permitted:
        # setpriv can retain permitted capabilities across its UID change. An
        # ordinary exec first clears them, so no_new_privs must then prevent the
        # file capability from adding them back. Log the shell's actual state.
        cmd.extend(["/bin/sh", "-c", 'while IFS= read -r line; do case "$line" in Cap*|NoNewPrivs:*) printf "%s\\n" "$line";; esac; done < /proc/self/status; exec "$@"', "sh"])
    cmd.append(str(binary))
    corefile = directory / f"{name}.Corefile"
    if flag:
        cmd.append(flag)
    else:
        extra = " reload 2s 0s\n" if reload_port else ""
        if health_port:
            extra += f" health {host}:{health_port}\n"
        if dnstap_port:
            if dnstap_tls:
                cert, key = directory / "cert.pem", directory / "key.pem"
                subprocess.run(["openssl", "req", "-x509", "-newkey", "rsa:2048", "-nodes",
                                "-keyout", str(key), "-out", str(cert), "-subj", "/CN=localhost", "-days", "1"],
                               check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
                key.chmod(0o644)
                extra += f" dnstap listen tls://{host}:{dnstap_port} {{\n  tls {cert} {key}\n }}\n"
            else:
                extra += f" dnstap listen tcp://{host}:{dnstap_port}\n"
        corefile.write_text(f".:{port} {{\n bind {host}\n{extra} template IN A {{\n  answer \"{{{{ .Name }}}} 60 IN A 192.0.2.1\"\n }}\n}}\n")
        corefile.chmod(0o644)
        cmd.extend(["-conf", str(corefile)])
    output = directory / f"{name}.log"
    error = None
    observed = False
    with output.open("w") as log:
        try:
            process = subprocess.Popen(cmd, stdout=log, stderr=subprocess.STDOUT)
            try:
                if flag:
                    observed = process.wait(timeout=8) == 0
                else:
                    deadline = time.monotonic() + 8
                    while process.poll() is None and time.monotonic() < deadline:
                        try:
                            query(host, port, False)
                            query(host, port, True)
                            observed = True
                            break
                        except (OSError, AssertionError):
                            time.sleep(0.05)
                    if observed and health_port:
                        connection = http.client.HTTPConnection(host, health_port, timeout=2)
                        try:
                            connection.request("GET", "/health")
                            response = connection.getresponse()
                            observed = response.status == 200 and response.read() == b"OK"
                        finally:
                            connection.close()
                    if observed and dnstap_port:
                        with socket.create_connection((host, dnstap_port), timeout=2) as connection:
                            if dnstap_tls:
                                context = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
                                # The test creates a throwaway, self-signed certificate.
                                context.check_hostname = False
                                context.verify_mode = ssl.CERT_NONE
                                with context.wrap_socket(connection, server_hostname="localhost"):
                                    pass
                    if observed and reload_port:
                        corefile.write_text(corefile.read_text().replace(f".:{port} {{", f".:{reload_port} {{"))
                        observed = False
                        deadline = time.monotonic() + 12
                        while process.poll() is None and time.monotonic() < deadline:
                            try:
                                query(host, reload_port, False)
                                query(host, reload_port, True)
                                observed = True
                                break
                            except (OSError, AssertionError):
                                time.sleep(0.05)
            finally:
                if process.poll() is None:
                    process.terminate()
                    try:
                        process.wait(timeout=5)
                    except subprocess.TimeoutExpired:
                        process.kill()
                        process.wait()
        except OSError as exc:
            error = str(exc)
            observed = False
    text = output.read_text()
    passed = observed == success
    if not success:
        passed = passed and "bind: permission denied" in text.lower()
    record = dict(case=name, expected_success=success, observed_success=observed, passed=passed, error=error, log=text.strip())
    print(json.dumps(record), flush=True)
    return passed


def main():
    parser = argparse.ArgumentParser()
    target = parser.add_mutually_exclusive_group(required=True)
    target.add_argument("--binary")
    target.add_argument("--image")
    parser.add_argument("--caps", default="")
    parser.add_argument("--label", required=True)
    args = parser.parse_args()
    if args.image:
        return test_image(args.image, args.label)
    assert os.geteuid() == 0, "run under sudo unshare --net"
    subprocess.run(["ip", "link", "set", "lo", "up"], check=True)
    with tempfile.TemporaryDirectory(prefix="coredns-capability-") as temp:
        directory = Path(temp)
        directory.chmod(0o755)
        binary = directory / "coredns"
        shutil.copyfile(args.binary, binary)
        binary.chmod(0o755)
        if args.caps:
            subprocess.run(["setcap", args.caps, str(binary)], check=True)
        print(json.dumps(dict(label=args.label, caps=args.caps, kernel=os.uname().release)), flush=True)
        cases = [
            dict(name="version-cap-drop-all", bounding="-all", threshold=1024, flag="-version"),
            dict(name="plugins-cap-drop-all", bounding="-all", threshold=1024, flag="-plugins"),
            dict(name="help-cap-drop-all", bounding="-all", threshold=1024, flag="-help"),
            dict(name="high-port-cap-drop-all", bounding="-all", threshold=1024, port=1053),
            dict(name="low-port-permitted", bounding="-all,+net_bind_service", threshold=1024, port=53),
            dict(name="low-port-relaxed-cap-drop-all", bounding="-all", threshold=0, port=53),
            dict(name="low-port-denied-control", bounding="-all", threshold=1024, port=53, success=False),
            dict(name="ipv6-low-port-permitted", bounding="-all,+net_bind_service", threshold=1024, port=53, host="::1"),
            dict(name="ipv6-high-port-cap-drop-all", bounding="-all", threshold=1024, port=1053, host="::1"),
            dict(name="custom-privileged-range", bounding="-all,+net_bind_service", threshold=2048, port=1053),
            dict(name="no-new-privs-high-port", bounding="-all", threshold=1024, port=1053, no_new_privs=True),
            dict(name="no-new-privs-relaxed-low-port", bounding="-all", threshold=0, port=53, no_new_privs=True),
            dict(name="no-new-privs-permitted-low-port", bounding="-all,+net_bind_service", threshold=1024, port=53, no_new_privs=True),
            dict(name="no-new-privs-denied-control", bounding="-all,+net_bind_service", threshold=1024, port=53, no_new_privs=True, clear_permitted=True, success=False),
            dict(name="root-low-port", bounding="-all,+net_bind_service", threshold=1024, port=53, uid=0),
            dict(name="reload-low-port", bounding="-all,+net_bind_service", threshold=1024, port=53, reload_port=54),
            dict(name="health-low-port", bounding="-all,+net_bind_service", threshold=1024, port=1053, health_port=81),
            dict(name="dnstap-low-port", bounding="-all,+net_bind_service", threshold=1024, port=1053, dnstap_port=82),
            dict(name="dnstap-tls-low-port", bounding="-all,+net_bind_service", threshold=1024, port=1053, dnstap_port=83, dnstap_tls=True),
        ]
        results = [run_case(binary, directory, **case) for case in cases]
        print(json.dumps(dict(label=args.label, passed=sum(results), total=len(results))), flush=True)
        return 0 if all(results) else 1


def test_image(image, label):
    results = []
    for name, port, threshold, permit, success in [
        ("permitted-low-port", 53, 1024, True, True),
        ("dropped-high-port", 1053, 1024, False, True),
        ("dropped-relaxed-low-port", 53, 0, False, True),
        ("dropped-denied-low-port", 53, 1024, False, False),
    ]:
        with tempfile.TemporaryDirectory(prefix="coredns-container-") as temp:
            corefile = Path(temp) / "Corefile"
            corefile.write_text(f".:{port} {{\n template IN A {{\n  answer \"{{{{ .Name }}}} 60 IN A 192.0.2.1\"\n }}\n}}\n")
            corefile.chmod(0o644)
            cmd = ["docker", "run", "-d", "--cap-drop=ALL",
                   "--sysctl", f"net.ipv4.ip_unprivileged_port_start={threshold}",
                   "-v", f"{corefile}:/Corefile:ro",
                   "-p", f"127.0.0.1::{port}/tcp", "-p", f"127.0.0.1::{port}/udp"]
            if permit:
                cmd.append("--cap-add=NET_BIND_SERVICE")
            cmd.extend([image, "-conf", "/Corefile"])
            container = subprocess.check_output(cmd, text=True).strip()
            observed = False
            try:
                ports = {}
                for transport in ("tcp", "udp"):
                    published = subprocess.run(["docker", "port", container, f"{port}/{transport}"], capture_output=True, text=True)
                    endpoint = published.stdout.strip()
                    if endpoint:
                        ports[transport] = int(endpoint.rsplit(":", 1)[1])
                deadline = time.monotonic() + 10
                while len(ports) == 2 and time.monotonic() < deadline:
                    state = json.loads(subprocess.check_output(["docker", "inspect", container], text=True))[0]["State"]
                    if not state["Running"]:
                        break
                    try:
                        query("127.0.0.1", ports["udp"], False)
                        query("127.0.0.1", ports["tcp"], True)
                        observed = True
                        break
                    except (OSError, AssertionError):
                        time.sleep(0.05)
                log = subprocess.check_output(["docker", "logs", container], stderr=subprocess.STDOUT, text=True)
                passed = observed == success
                if not success:
                    passed = passed and "bind: permission denied" in log.lower()
                results.append(passed)
                print(json.dumps(dict(case=name, expected_success=success, observed_success=observed, passed=passed, log=log.strip())), flush=True)
            finally:
                subprocess.run(["docker", "rm", "-f", container], check=True, stdout=subprocess.DEVNULL)
    print(json.dumps(dict(label=label, passed=sum(results), total=len(results))), flush=True)
    return 0 if all(results) else 1


if __name__ == "__main__":
    raise SystemExit(main())
