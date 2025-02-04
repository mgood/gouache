for d in *.ink; do
    echo $d.txt
done |
xargs redo-ifchange
