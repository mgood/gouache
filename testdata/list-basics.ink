/*
	Takes a list and prints it out, using commas.

listDefs should declare globals
set "parent.name" = value
if name is unique, set that global name too

*/

LIST volumeLevel = off, quiet, medium, loud, deafening
~ temp x = loud
{x}
{x == medium}
{(off, loud) == (medium)}
{x != off}

{LIST_ALL(volumeLevel)}

Adding items
~ x += quiet
~ x += (off, medium)
{x}

Adding 1 will increment all items
{x + 1}

Adding 2 will push "loud" to be undefined, and drop
it from the list
{x + 2}

Subtracting
{x - 1}
{x - (off, quiet)}

{LIST_COUNT(x)}
a single value still counts as a 1-item list
{LIST_COUNT(quiet)}

Look up a list item based on numeric value
{volumeLevel(2)}

empty {LIST_VALUE(())}
{LIST_VALUE(deafening)}
multiple items takes largest value
{LIST_VALUE(off+quiet)}

LIST fruitBowl = (apples), (bananas), (oranges)
{fruitBowl}
{LIST_MIN(fruitBowl)}
{LIST_MAX(fruitBowl)}

~ temp combo = (apples, oranges, quiet, medium)
{combo}
~ combo += bananas
{combo}

{():has values|is empty}
{combo:has values|is empty}

~ temp fruit = (apples, oranges)

{fruit ? (apples, oranges):
	? has apples and oranges
}
{fruit has apples:
	has apples
}
{fruit has (apples, bananas):
	fruit has apples and bananas
}
{fruit has ()}
{() has ()}

{fruit !? (apples, bananas):
	hasnt apples and bananas
}
{fruit hasnt apples:
	hasnt apples and bananas
}
{() hasnt ()}

{listWithCommas(fruitBowl, "empty")}

{LIST_ALL((off, apples))}

~ temp r = (off, apples)
{LIST_ALL(r)}
~ r -= off
{LIST_ALL(r)}
~ r -= apples
r ={r}
r all {LIST_ALL(r)}
{LIST_ALL((off, apples) - (off, apples))}

=== function listWithCommas(list, if_empty)
    {LIST_COUNT(list):
    - 2:
        	{LIST_MIN(list)}, and {listWithCommas(list - LIST_MIN(list), if_empty)}
    - 1:
        	{list}
    - 0:
			{if_empty}
    - else:
      		{LIST_MIN(list)}, {listWithCommas(list - LIST_MIN(list), if_empty)}
    }
