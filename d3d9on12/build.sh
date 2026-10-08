#!/bin/sh
# Builds the 32-bit d3d9.dll proxy with clang + lld-link (no MSVC needed).
set -e
cd "$(dirname "$0")"
llvm-dlltool -m i386 -d kernel32.def -l kernel32.lib -k
llvm-dlltool -m i386 -d user32.def -l user32.lib
clang --target=i686-pc-windows-msvc -O2 -fms-extensions -fasm-blocks -fno-builtin -fno-stack-protector -c proxy.c -o proxy.obj
lld-link /export:Direct3DCreate9=_Direct3DCreate9@4 /export:Direct3DCreate9Ex=_Direct3DCreate9Ex@8 /dll /nodefaultlib /entry:DllMain@12 /safeseh:no /machine:x86 /out:d3d9.dll proxy.obj kernel32.lib user32.lib
