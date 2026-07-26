# Videos are previewed as three frames, not played

A video is previewed by extracting frames at 25%, 50%, and 75% of its duration via `ffmpeg` and rendering all three inline; images are rendered directly. Playback in a terminal would mean either a heavy dependency or a poor imitation of one, and it costs real time per file. Three frames spread across the clip are enough to identify what a recording is, which is the only job the preview has, and they render instantly as static images.

## Consequences

Content identifiable only from motion — distinguishing two similar movements, say — may not be recognizable from three stills.
