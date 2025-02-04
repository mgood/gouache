for d in *.ink; do
    echo $d.json
done |
xargs redo-ifchange

for d in *.ink.json; do
    echo ${d%.json}.txt
done |
xargs redo-ifchange
