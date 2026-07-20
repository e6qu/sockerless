# Keep the newest complete release tags and their two architecture-specific
# images. Untagged versions, incomplete releases, and versions containing any
# other tag are obsolete.

([ .[].metadata.container.tags[]? ] | unique) as $all_tags
| ([ .[] as $version
     | ($version.metadata.container.tags // [])[]
     | select(test("^[0-9a-f]{12}$"))
     | {tag: ., created_at: $version.created_at}
   ]
   | unique_by(.tag)
   | map(select(
       .tag as $tag
       | ($all_tags | index($tag + "-amd64")) != null
         and ($all_tags | index($tag + "-arm64")) != null
     ))
   | sort_by(.created_at)
   | reverse
   | .[:$keep]
   | map(.tag)
  ) as $release_tags
| ($release_tags
   | map(., . + "-amd64", . + "-arm64")
   | unique
  ) as $keep_tags
| .[]
| . as $version
| ($version.metadata.container.tags // []) as $tags
| select(
    ($tags | length) == 0
    or any($tags[]; . as $tag | $keep_tags | index($tag) == null)
  )
| $version.id
