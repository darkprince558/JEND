# AI Assistant Rules & Guidelines

- **Do NOT break features for the sake of tests:** Never break or degrade user-facing features (like IPv6 support, cross-platform compatibility, etc.) just to make CI/CD pipelines or local tests pass.
- **Think broadly:** Always consider the broader architectural impact and the real-world end-user experience before pushing fixes.
- **Tests serve features:** Tests should validate the features; features shouldn't be compromised to appease the tests. If a test is failing because it doesn't account for a valid feature (like IPv6 listening), fix or adjust the test/environment rather than deleting the feature.
