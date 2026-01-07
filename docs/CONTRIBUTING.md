# How to contribute to JAAS documentation

Thanks for your interest in JAAS documentation -- contributions like yours make good projects
great!

There are two basic ways to contribute: by opening
an issue or by creating a PR. This document gives detailed information about
both.

> Note: If at any point you get stuck, come chat with us on
[Matrix](https://matrix.to/#/#jimm:ubuntu.com).

## Open an issue

You will need a GitHub account ([sign up](https://github.com/signup)).

### Open an issue for docs

To open an issue for a specific doc, in [the published docs](https://documentation.ubuntu.com/jaas/latest/) find the doc, then use the **Give feedback** button.

To open an issue for docs in general, do the same for the homepage of the docs
or go to https://github.com/canonical/jaas-documentation/issues, click on **New issue** (top right corner
of the page), select “Blank issue”, then fill out the issue template and
submit the issue.

### Open an issue for code

Go to https://github.com/canonical/jimm/issues click on **New issue** (top right
corner of the page), select whatever is appropriate, then fill out the issue
template and submit the issue.

## Make your first contribution

You will need a GitHub account ([sign up](https://github.com/signup) and [add
your public SSH key](https://github.com/settings/ssh)) and `git` ([get
started](https://git-scm.com/book/en/v2/Getting-Started-What-is-Git%3F)).

Then:

1. [Sign the Canonical Contributor Licence Agreement
   (CLA)](https://ubuntu.com/legal/contributors).

2. Configure your `git` so your commits are signed:

```
git config --global user.name "A. Hacker"
git config --global user.email "a.hacker@example.com"
git config --global commit.gpgsign true
```

> See more: [GitHub | Authentication > Signing commits](https://docs.github.com/en/authentication/managing-commit-signature-verification/signing-commits)

3. Fork canonical/jimm. This will create `https://github.com/<user>/jimm`.

4. Clone your fork locally.

```
git clone git@github.com:<user>/jaas-documentation.git
cd jaas-documentation
```

5. Add a new remote with the name `upstream` and set it to point to the upstream
`jimm` repo.

```
git remote add upstream git@github.com:canonical/jimm.git
```

6. Set your local branches to track the `upstream` remote (not your fork). E.g.,

```
git fetch --all
git checkout v3
git branch --set-upstream-to=upstream/v3
```

7. Sync your local branches with the upstream, then check out the branch you
want to contribute to and create a feature branch based on it.

```
git fetch upstream
git checkout v3
git pull
git checkout -b v3-new-stuff # your feature branch
```

8. Make the desired changes: All changes should follow the existing patterns, including
[Diátaxis](https://diataxis.fr), the [Canonical Documentation Style
Guide](https://docs.ubuntu.com/styleguide/en), the modular structure, the
cross-referencing pattern, [MyST
Markdown](https://canonical-starter-pack.readthedocs-hosted.com/latest/reference/myst-syntax-reference/),
etc.

Test changes locally: Changes should be inspected by building the docs and fixing any issues
discovered that way. To preview the docs as they will be rendered on RTD, run
`make run` and open the provided link in a browser. If you get errors, try `make
clean`, then `make run` again. For other checks, see `make [Tab]` and select the
command for the desired check.

9. As you make your changes, ensure that you always remain in sync with the upstream:

```
git pull upstream v3 --rebase
git push --force
```

10. Stage, commit and push regularly to your fork. Make sure your commit messages
comply with conventional commits ([see upstream
standard](https://www.conventionalcommits.org/en/v1.0.0/)). E.g.,

```
git add .
git commit -m "docs: add setup and teardown anchors"
git push origin v3-new-stuff
```

> Tip: If you've set things up correctly, typing just `git push` and returning
may be enough for `git` to prompt you with the correct arguments.

> Tip: If you don't want to create a new commit message every time, do
`git commit --amend --no-edit`, then `git push --force`. However, be careful with this,
as it overwrites your commit.

11. Create the PR.

12. Ensure GitHub tests pass.

13. In [the Matrix JIMM
channel](https://matrix.to/#/#jimm:ubuntu.com), drop a link to your
PR with the mention that it needs reviews. Someone will review your PR. Make all
the requested changes.

14. When you've received one approval, your PR can be merged. Depending on your
repo access, at this point you may be able to merge directly (by clicking on
Merge pull request). Otherwise, ping one of your reviewers and ask them to help
merge.

> Tip: After your first contribution, you will only have to repeat steps 7-14.

Congratulations and thank you!


