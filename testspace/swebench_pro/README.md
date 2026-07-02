# SWE-bench Pro samples

Source dataset: `ScaleAI/SWE-bench_Pro`

This directory contains a small local sample of 5 cases from the SWE-bench Pro `test` split.

## Files

- `first5.json`: sample metadata used to pick the cases
- `repos/`: local repository checkouts, one directory per `instance_id`

Each sampled repository is checked out at the exact `base_commit` from the dataset.

## Cases

1. `instance_NodeBB__NodeBB-04998908ba6721d64eba79ae3b65a351dcfbc5b5-vnan`
2. `instance_qutebrowser__qutebrowser-f91ace96223cac8161c16dd061907e138fe85111-v059c6fdc75567943479b23ebca7c07b5e9a7f34c`
3. `instance_NodeBB__NodeBB-51d8f3b195bddb13a13ddc0de110722774d9bb1b-vf2cf3cbd463b7ad942381f1c6d077626485a1e9e`
4. `instance_qutebrowser__qutebrowser-c580ebf0801e5a3ecabc54f327498bb753c6d5f2-v2ef375ac784985212b1805e1d0431dc8f1b3c171`
5. `instance_ansible__ansible-f327e65d11bb905ed9f15996024f857a95592629-vba6da65a0f3baefda7a058ebbd0a8dcafb8512f5`

## Manual run

From the repo root:

```bash
cd testspace/swebench_pro/repos/<instance_id>
git status --short
printf '%s\n' '<problem_statement>' | ../../../../5hagent
```

Example:

```bash
cd testspace/swebench_pro/repos/instance_ansible__ansible-f327e65d11bb905ed9f15996024f857a95592629-vba6da65a0f3baefda7a058ebbd0a8dcafb8512f5
git status --short
printf '%s\n' 'The current validation system for Fully Qualified Collection Names (FQCN) in ansible-galaxy incorrectly accepts collection names that contain Python reserved keywords. Reject names that contain Python reserved keywords in either the namespace or collection name portion.' | ../../../../5hagent
```

## Inspect result

After the run:

```bash
git status --short
git diff
```

If you want to discard the local test changes and return to the sampled base commit:

```bash
git restore .
git clean -fd
```

## Notes

- These repositories are in detached HEAD state on purpose.
- `dockerhub_tag` is preserved in `first5.json` in case you later want to align with the official evaluation images.
- This directory does not yet include an automated runner for these 5 Pro cases.
