These synthetic fixtures contain no recorded speech or third-party audio.

`legacy-mono.flac` was generated once with the previous FFmpeg 8.0.1 build to
verify compatibility with existing recordings. Its 16,037 mono samples at
16 kHz are `(i*7919+12345)%65536-32768` (signed 16-bit PCM).

`reference-stereo.flac` and `reference-24bit.flac` were generated once with
Xiph's reference `flac 1.5.0 --verify` at default compression. Both contain a
one-second 440 Hz sine. The stereo fixture is 48 kHz, signed 16-bit, amplitudes
0.4/0.2. The mono fixture is 44.1 kHz, signed 24-bit, amplitude 0.4. Samples are
rounded to the nearest integer after multiplying by `2^(bits-1)-1`.

`manifest.json` records encoded-file and original interleaved PCM SHA-256 hashes.
Running tests needs neither encoder executable nor any native codec library.
