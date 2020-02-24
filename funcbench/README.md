# funcbench

A tool used as a github action to run a `go test -bench` and compare changes from a PR against another branch or commit. 

The benchmark can be triggered by creating a comment which specifies a branch to compare. The results are then posted back as a PR comment.
The benchmark can be also trigger and used as CLI command, without GitHub hook.

Comparison is done using [benchcmp](https://godoc.org/golang.org/x/tools/cmd/benchcmp). 
Arguments for the benchcmp are read from files created by previous action (for example [commentMonitor](/tools/commentMonitor/main.go)),
which is responsible for the comment parsing.

## Usage

Examples:

* Execute benchmark named `FuncName` regex, and compare it with `master` branch.

 ```
 /funcbench master BenchmarkFuncName
 ```

* Execute all benchmarks matching `FuncName.*` regex, and compare it with `master` branch.
 
```
 /funcbench master FuncName.*
 ```

* Execute all benchmarks, and compare the results with `devel` branch.

 ```
 /funcbench devel .
 ```

* Execute all benchmarks matching `FuncName.*` regex, and compare it with `6d280faa16bfca1f26fa426d863afbb564c063d1` commit.

 ```
 /funcbench 6d280faa16bfca1f26fa426d863afbb564c063d1 FuncName.*
 ```

* Execute all benchmarks matching `FuncName.*` regex on current code. Compare it between sub-benchmarks (`b.Run`) of same benchmark for current commit.
Errors out if there are no sub-benchmarks.

 ```
 /funcbench . FuncName.*
 ```

### CLI without Github on existing local repository.

You can run it locally without any webhook. Just pass `--local/-l` flag e.g:

Execute all benchmarks matching `FuncName.*` regex, and compare it with `master` branch without GitHub notification. 
Use local repository for that. NOTE: Clean git state is required.
 
```
./funcbench --local master FuncName.*
 ```


### GitHub 

Tests are triggered by posting a comment in a PR with the following format:

`/funcbench <branch> <Go test regex> [-no-race]`

Specifying which tests to run are filtered by using the standard [Go regex RE2 language](https://github.com/google/re2/wiki/Syntax).

By default all benchmarks run with `-race` flag enabled and it can be disabled by appending `-no-race` at the end of the comment.

#### Example Github actions workflow file to pass in --input flag.

```
on: issue_comment // Workflow is executed when a pull request comment is created.
name: Benchmark
jobs:
  commentMonitor:
    runs-on: ubuntu-latest
    steps:
    - name: commentMonitor
      uses: docker://prombench/comment-monitor:latest
      env:
        COMMENT_TEMPLATE: 'The benchmark has started.' // Body of a comment that is created to announce start of a benchmark.
        GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }} // Github secret token/
      with:
        args: '"^/funcbench ?(?P<BRANCH>[^ B\.]+)? ?(?P<REGEX>\.|Bench.*|[^ ]+)? ?(?P<RACE>-no-race)?.*$"'
    - name: benchmark
      uses: docker://prombench/funcbench:latest
      env:
        GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }} // Github secret token/
```

#### Set up

This tools is meant to be used with a Github action. The action itself is, to a large degree, unusable alone, as you need
to combine it with another Github action that will provide necessary files to it. At this time, the only action it is
supposed to work with, is [comment-monitor](https://github.com/prometheus/prombench/tree/master/tools/commentMonitor).

- Create Github actions workflow file that is executed when an issue comment is created, `on = "issue_comment"`.
- Add comment-monitor Github action as a first step.
- Specify this regex `^/funcbench ?(?P<BRANCH>[^ B\.]+)? ?(?P<REGEX>\.|Bench.*|[^ ]+)? ?(?P<RACE>-no-race)?.*$` in the `args` field of the comment-monitor.
- Specify this Github action as a pre-built image, build from this source code, or just refer to this repository from the workflow file.
- Provide a Github token as an environment variable to both comment-monitor and funcbench.

## Building Docker container.

From the repository root:

`docker build -t <tag of your choice> -f funcbench/Dockerfile .`