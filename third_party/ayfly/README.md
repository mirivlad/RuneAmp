# AY/YM module replay for RuneAmp

Adapted PT1, PT2, STP, ASC, PSC, SQT and STC replay routines from Ayfly
(Copyright 2008 Deryabin Andrew; routines based on AY-3-8910/12 Emulator,
Copyright 1999–2004 S. V. Bulba):
https://github.com/l29ah/ayfly/tree/c1ff6d559edc24ec43c7df0bf23a4531b976133d
GPL-2.0-or-later. RuneAmp distributes these modifications under GPL-3.0.
The tone tables originate in PT1Play.h and STCPlay.h. Changes: isolated
static single-track state, bounded command loops and table lookups, first-loop
end notification, PT2 relative-offset validation, ASC0 header normalization
and validation of used pattern pointers. Legacy replay is isolated in a worker;
it is not a fully verified parser for every possible module.

registers.h contains a bounded canonical-Huffman LH5 decoder and YM/VTX
register-dump readers; PSG commands are decoded in bridge.cpp. YM5/6 digital
effects and sampled drums are not supported. Ayumi synthesizer is reused from
third_party/pt3 (MIT).

Build: clang and clang++ with wasm-ld, ./third_party/ayfly/build.sh.
Test: node third_party/ayfly/test.cjs [directory of test modules] [--full].
Real samples used for validation were from
https://github.com/vitamin-caig/zxtune/tree/736bf6006bb86993b3b256ac7fd47a572f8def19/samples/chiptunes/AY-3-8910
and are not distributed with RuneAmp. No songs are included in tests.
