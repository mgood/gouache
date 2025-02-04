redo-ifchange $2
inklecate -o $3 $2
jq . $3 > $3.formatted
mv $3.formatted $3
