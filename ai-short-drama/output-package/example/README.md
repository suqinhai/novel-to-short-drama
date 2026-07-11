# Manual package example

This directory documents the package written at runtime under
`storage/output-packages/<project_id>/<episode_id>/<platform>/<metadata_version>/`.
Media placeholders are deliberately not committed: a zero-byte file must never
be mistaken for a rendered master.

Expected runtime tree:

```text
final.mp4
clean.mp4                 # when GENERATE_CLEAN_MASTER=true
subtitles.srt
subtitles.ass             # when ASS generation is enabled
cover.jpg
metadata.json
qc-report.json
upload-instructions.txt
```

The `manual_package` provider validates every required file before returning a
package URL. If a rendered master or cover is missing, it returns
`manual_required` with a non-success package validation result; it never reports
the episode as published.
