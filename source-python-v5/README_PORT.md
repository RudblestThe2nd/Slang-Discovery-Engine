# Preserved v5 Python research implementation

This folder is provided as **source code reference**, not as the engine used by the packaged Windows .exe. It retains the original Python NLP prototypes and their own requirements/tests for optional experimentation. Some parts depend on third-party packages and permitted Reddit API access.

The Windows app in `../app/` is a standalone Go implementation with a lightweight lexical/phrase/context-change hypothesis finder. The two engines need not produce identical candidates, scores, or DB formats.
