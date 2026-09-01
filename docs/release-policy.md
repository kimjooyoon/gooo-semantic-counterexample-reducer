# Release policy

The initial bootstrap commit is retained as the sole `BOOTSTRAP_EXCEPTION`. Feature
changes must be merged through a pull request. The release workflow accepts an exact
merged commit SHA and refuses a reused version or tag.

Each release contains a source archive, the two reduction reports, CI metrics, a version
record, a release manifest, and `SHA256SUMS`. The workflow verifies the annotated tag's
target, release asset sizes and digests, and the GitHub release immutability field before
uploading an audit artifact.
