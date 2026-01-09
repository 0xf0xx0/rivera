# git-rivera/git-河流

[git-foresta](https://github.com/takaaki-kasai/git-foresta) in go

![screenshot of git-rivera output, printing cli-pride-flags](./showoff.png)

## features/differences from git-foresta

- persistent branch coloring
- `--repository` option to pass a path (no more needing to cd beforepaw)
- a bit more control over overpasses (no more ODODO -> OOOOO)

## install

```sh
go install git.0xf0xx0.eth.limo/0xf0xx0/rivera@latest
```

### from source

```sh
git clone https://git.0xf0xx0.eth.limo/0xf0xx0/rivera.git
cd rivera
go build
go install
```

## usage

with git:

```sh
git rivera
```

or use it standalone:

```sh
git-rivera
```

## options

```
    --repository path, --repo path                              repository path to use (default: ".")
    --hashlength len, --hashlen len, -l len                     length of the commit hash (default: 8)
    --style num, -s num                                         style num to select (1-5) (default: 1)
    --subvinedepth uint, --svdepth uint, --depth uint, -d uint  maximum length of merge subvines (default: 2)
    --graphmarginleft uint, --marginl uint                      left margin of the commit graph (default: 2)
    --graphmarginright uint, --marginr uint                     right margin of the commit graph (default: 1)
    --all, -a                                                   display all branches
    --reverse, -r                                               reverse the flow
    --branchcolors color,color[,color]                          comma separated color,color[,color] used for branches, passed straight to oigiki (default: "red, blue, yellow, green, cyan, magenta, white")
    --maxgitrecursedepth uint, --mgrd uint                      maximum depth to search for a .git dir (default: 8)
    --help, -h                                                  show help
    --version, -v                                               print the version
    --color                                                     force color output
    --nocolor, --stdout                                         disable color output
```

## todo

- finish compat
