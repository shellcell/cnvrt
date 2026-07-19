# bash completion for cnvrt

_cnvrt() {
    local cur prev commands flags
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"
    commands="doctor formats backends config add-format add-tool help version"
    flags="-i --input-format -o --output-format --out-dir --overwrite --quality --action --compress --resize --opt -h --help -v --version"

    case "$prev" in
        -i|--input-format|-o|--output-format|--quality|--resize|--opt)
            return
            ;;
        --action)
            COMPREPLY=($(compgen -W "convert compress resize" -- "$cur"))
            return
            ;;
        --out-dir)
            COMPREPLY=($(compgen -d -- "$cur"))
            return
            ;;
    esac

    if [[ $cur == -* ]]; then
        COMPREPLY=($(compgen -W "$flags" -- "$cur"))
        return
    fi

    if [[ $COMP_CWORD -eq 1 ]]; then
        COMPREPLY=($(compgen -W "$commands" -- "$cur") $(compgen -f -- "$cur"))
        return
    fi

    COMPREPLY=($(compgen -f -- "$cur"))
}

complete -o filenames -F _cnvrt cnvrt
