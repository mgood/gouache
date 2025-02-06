redo-ifchange $2
err=`inklecate -o $3 $2`
if [ -n "$err" ]; then
	echo "$err" >&2
	exit 1
fi
jq . $3 > $3.formatted
mv $3.formatted $3
