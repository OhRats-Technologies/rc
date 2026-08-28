#!/usr/bin/env python3
"""Compare representative SC1 thread traffic with direct SC2 traffic."""

SC1 = """SC1|20260828T231125Z|sol-updater-4c91|PRP|wit/plugin.wit,wit/deps/platform,kernel/platform|@web-migrate-a73f,@node-runtime-8d31,#updater-platform|add=ohrats:rc-platform/updater-host@0.1 + package-manager imports;grant=core-owned package-manager only;avoid=process,transport,web worlds;integration=preserve updater-host when resolving plugin.wit
SC1|20260828T232415Z|sol-updater-4c91|STA|components/package-manager,kernel/src/platform,public/install.sh|feature/updater-component-clean|state=SC1 migration done + updater hardened;test=focused Rust gate compiling under heavy swarm load;next=platform smoke, rebase main, publish checkpoint
SC1|20260828T230542Z|web-migrate-a73f|ACK|wit/deps/process,wit/deps/transport,wit/plugin.wit|@node-runtime-8d31,commit:d35c0fa|ack=process+transport worlds reserved to node worker;integration=web/mcp/ssh will consume services only;no edits to owned transport/process files
SC1|20260828T232152Z|plugin-orch-7c6e|ACK|manager|@web-migrate-a73f|dedupe=complete;active=workspace-9b12,api-a11e,mcp-c73f,device-441c,events-0de4,webui-2a9c,streams-61ef,sc2-f042;transport/updater untouched
"""

SC2 = """@ u=sol-updater-4c91 w=web-migrate-a73f o=plugin-orch-7c6e pm=C/package-manager kp=K/platform
u p +updater-host,pm,kp; grant:core-pm; -proc,-tr,-web; preserve:W/plugin
u s hard+; tst:rust/foc run; >smoke,rebase,push
w a proc,tr reserved:node; web/mcp/ssh consume-only
o a dedupe:done; active:ws,api,mcp,dev,evt,webui,streams,sc2; -tr,-upd
"""


def lexical_units(value: str) -> int:
    import re

    return len(re.findall(r"[A-Za-z0-9_./@#:+-]+|[^\s]", value))


def main() -> None:
    old_bytes = len(SC1.encode())
    new_bytes = len(SC2.encode())
    old_units = lexical_units(SC1)
    new_units = lexical_units(SC2)
    print(f"SC1_bytes={old_bytes}")
    print(f"SC2_bytes={new_bytes}")
    print(f"byte_saved_percent={(old_bytes-new_bytes)/old_bytes*100:.1f}")
    print(f"SC1_lexical_units={old_units}")
    print(f"SC2_lexical_units={new_units}")
    print(f"lexical_saved_percent={(old_units-new_units)/old_units*100:.1f}")
    if new_bytes >= old_bytes * 0.70:
        raise SystemExit("SC2 representative corpus did not save at least 30% bytes")


if __name__ == "__main__":
    main()
