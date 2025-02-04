redo-ifchange $2.json
# use head -n100 to limit the number of choices in case we hit an infinite loop
yes 1 | head -n 100 | inklecate -p $2.json
