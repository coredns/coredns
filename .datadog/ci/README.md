## Docker image used for CI
To update the image:

1. Update the Dockerfile
1. Test that it can be built locally with `bash release-image.sh`
1. Commit and push your changes (this ensures that we have the Dockerfile for
	 the current image somewhere in git)
1. Run `bash release-image.sh --push`, which will output the tag of the new image.
1. Update the `CI_IMAGE` variable in `.gitlab-ci.yml` to reference the new image
   tag
1. Commit and push your changes, create a PR, and merge to the default branch after review.