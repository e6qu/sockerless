# Sockerless - What We Built

Roadmap [PLAN.md](PLAN.md) - status [STATUS.md](STATUS.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md) - coverage [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md).

Detailed historical narrative lives in PR descriptions and `git log`. This file keeps the recent chain plus a compact foundation summary.

## 2026-08-06 — A response member's pattern is part of the contract

The runtime spec validator checked that a response member was a string and
stopped there. A `smithy.api#pattern` is as much of the wire contract as the
type is, and it is where the identity-bearing strings are pinned — an ARN, an
instance id, a lock token — so a value that deserialized fine could still be one
no client could ever send back. That is how a malformed ARN reached every AWS
Organizations response and went unnoticed until it was looked for.

`validateSmithyValue` now matches a string shape's pattern, compiled once per
shape. Smithy patterns are unanchored unless the expression anchors itself,
which is exactly what `MatchString` does; a pattern the Go engine cannot compile
constrains nothing rather than failing every value, because the validator
refuses to judge what it cannot read.

Arming it reported 185 divergences over 87 keys. All but three are fixed rather
than allowlisted.

AWS WAFV2 accounted for 43 of them and now for none. It emitted a lock token
that was not a UUID though the model types it exactly as it types an entity id,
seeded a rule set whose id read `EXAMPLE`, and returned a created entity's
Description as `""` where the service omits the member — the stored shapes
carried `omitempty` but the summary was built as a map, which has no such thing.
It also accepted entity names the service rejects and echoed them back; it now
refuses them the way AWS does, which is what had let them into responses at all.

AWS Systems Manager accounted for 26. VersionName, TargetType and CaptureTime
were emitted as `""`, a reviewer was reported as an ARN where the member admits
no colon, and an access-request id was built as `oar-` plus seventeen characters
where the model says `oi-` plus twelve. SendCommand now applies AWS's documented
MaxConcurrency and MaxErrors defaults rather than returning empty strings, which
is what the service does with a request that names neither.

The rest were singletons. Amazon CloudWatch Logs carried a UUID's hyphens into
an alphanumeric delivery id and reported an OpenSearch endpoint as `""` — the
comment beside it said it would not fabricate the fact, which was the right
instinct and the wrong execution, since an empty string is itself a value the
model forbids. AWS CloudTrail returned a UUID where a refresh id is decimal. AWS
KMS returned a dashed UUID where a key-material id is hex. A managed Contributor
Insights rule was created with an empty definition, and a rule's definition is
what the rule is, so it now carries the body it actually runs. AWS Glue emitted
an absent iterable form name as `""`.

Several were not the simulator at all but test inputs no real caller could send:
instance ids sixteen characters long where the model admits eight or seventeen,
a lookup-table name with hyphens, an Amazon S3 ARN where an S3 URI belongs, a
Route 53 hosted-zone id carried into AWS Certificate Manager with its
`/hostedzone/` prefix still on it, and AWS WAFV2 names built from a
fractional-seconds timestamp. Each is fixed where a real client would have had
to get it right.

Three are allowlisted, tracked as BUG-2932, because the pattern is stricter than
the service it describes: Amazon EventBridge really does name a connection's
managed secret `events!connection/...`, AWS Certificate Manager really does
report an AWS Private Certificate Authority ARN as the issuing authority, and
Amazon CloudWatch Logs really does report a configuration template's resource
type in CloudFormation spelling. Emitting something the pattern admits would
mean emitting something AWS never emits. They close when a model revision
widens the pattern, not when the simulator changes.

The check carries its own regression rather than resting on the suites it
serves, and the AWS simulator now has a violation allowlist where it had none,
so any new pattern divergence fails CI.

## 2026-08-06 — AWS Organizations reads the resource type off the identifier

AWS Organizations is the service whose identifiers say what they identify. Every
id the model declares carries a literal prefix — `r-` a root, `ou-` an
organizational unit, `p-` a policy, `h-` a handshake, `rp-` a resource policy,
`rt-` a responsibility transfer, and a bare twelve digits an account — and the
members that accept more than one kind are alternations over exactly those
prefixes: `TaggableResourceId` admits six, `PolicyTargetId` three, `ParentId`
two. Ten of its operations name their resource under a member whose own name
says nothing about the type — `TargetId`, `ParentId`, `ChildId`, `ResourceId` —
so a derivation keyed on field spellings could only guess. The gate reads the
type off the identifier, because that is the only way a caller can state it and
therefore the way AWS reads it too.

The ARN is then the resource's own. Four types carry an attribute no request
supplies — a policy's type and whether AWS manages it, what a handshake is for,
what a transfer moves and which way, and a resource policy's assigned id — and
those are resolved through the simulator's state, as Amazon RDS resolves a
custom engine version. The rest are a function of the identifier alone and are
built from it, so a request naming a resource that does not exist is still
authorized against the ARN it names and then reported missing, rather than
refused as unauthorized.

Thirty-eight of the forty operations derive. `CreatePolicy` names a policy that
does not exist yet, and `DescribeEffectivePolicy` takes a target that may be a
root, an organizational unit or an account while the reference declares only the
account — so its two other spellings have no declared type to authorize against.
Coverage went from 1,423 to 1,461 of 1,974 served operations.

Six defects in the service itself came out with it, each a shape the model
constrains with a `smithy.api#pattern` and nothing checked. The organization id
was one character short of `OrganizationId`, which made every ARN Organizations
emits malformed. `p-FullAWSAccess` was built the customer way, though only the
AWS-managed arm of `PolicyArn` admits its uppercase letters. Every handshake
claimed to be an invitation, so an enable-all-features handshake and a
responsibility-transfer handshake — which was also labelled `INVITE` rather than
`TRANSFER_RESPONSIBILITY` — were indistinguishable from one. A transfer's ARN
used the resource-type name `responsibilitytransfer` where the ARN segment is
`transfer`, and omitted what moves and which way. A hierarchy path was reported
from the node upwards without the organization or the trailing slash, inverting
what a path means. And an organization id was placed in a member modeled as an
account id, where inviting an organization names no account at all.

The tests hold each ARN to the pattern the vendored model declares rather than
to a spelling chosen here, drive the simulator's own API end to end, and assert
the ARN contract at the official-SDK surface too, which had checked id prefixes
and never ARNs. Every fix was confirmed by a negative control that fails with it
reverted.

What found them is filed as BUG-2931: the runtime spec validator checks a
response member's type and not its `smithy.api#pattern`, so a value that
deserializes as a string passes however it is spelled. Arming that check reports
185 divergences over 87 keys across seven more services, which is a burn-down of
its own rather than this change's subject.

## 2026-08-05 — CreateSecret names its secret in Name, not SecretId

Reported as GitHub issue #889 and consolidated into the same branch as the
Amazon RDS ARN work, because only one pull request is open at a time.

The gate read an AWS Secrets Manager request's resource from `SecretId`.
`CreateSecret` does not send one — it names the secret it is about to create in
`Name` — so the lookup missed, the request fell through to a literal `"*"`, and
a role holding exactly the grant it needed was denied against a policy that
plainly granted it. It is the same shape as the defect that started this work:
an action whose resource the gate cannot read is authorized against `"*"`, which
matches only a policy whose Resource is itself `"*"`, so the scoped grant is the
one that fails.

A crash found on the way, and it was not the branch's doing but the branch's
CI run is what surfaced it. The simulator could panic at startup with
`route53 dns: bind ... address already in use`, taking every test in the shard
with it — they waited on a server that never came up.

Route 53 answers DNS on UDP and TCP, and a resolver that gets a truncated UDP
answer retries the query over TCP on the same port, so the server needs one port
on both. Asking the kernel for a free port asks it about one protocol: the two
spaces are independent, so the UDP port it hands out says nothing about whether
that TCP port is free, and on a busy host it often is not. An ephemeral port is
retried now until both protocols answer to the same number, bounded so a host
where none is ever free is reported rather than asked forever; a port the
operator configured still fails on the first attempt, because then the address
is the request and serving DNS somewhere else would be worse than not starting.

The first test written for it was worthless and the negative control said so: it
held a TCP port and hoped the kernel would hand out that same UDP port, which it
had no reason to do, and it passed with the retry removed. The single attempt is
now injectable and the retry driven directly, so removing the retry fails the
test.

AWS published `SearchVectors` into the Amazon DynamoDB Service Reference while
this branch was in flight, a few hours after the operation was implemented here.
It declares the index and the table, which the request names both of, so it
derives the moment the reference is refreshed — a service gaining an action
needs no code when the derivation is driven by what AWS publishes. The
assertion that the reference still lists neither `TransactWriteItems` nor
`TransactGetItems` was checked against the new document and holds.

## 2026-08-05 — AWS KMS, and a prefix carried once

AWS KMS derives its two resource types and gives up its per-service case, taking
the gate to 1,423 of 1,973 served operations across fifteen services.

A key needed nothing: it is named by KeyId, bare or already an ARN, and the
shared derivation passes an ARN through as it stands.

An alias needed one thing said out loud. Its ARN is `alias/${Alias}`, and the
API's AliasName already carries that prefix — an alias is created and deleted as
"alias/my-key", not "my-key". Filling the published format with the name
unchanged would have produced `alias/alias/my-key`, which names nothing, and the
gate would have denied every policy written against the real alias while looking
like it was deriving properly. The prefix is stripped once so the ARN carries it
once, and a name given without it still yields the same single ARN, so the two
spellings a caller might use do not become two resources.

That hazard is why AWS Organizations is not in this change despite being the
next by size. Its ARNs embed literal prefixes the same way —
`policy/o-${OrganizationId}/${PolicyType}/p-${PolicyId}` — where the API's
PolicyId already carries the `p-`, and the organization id is not in the request
at all. It needs the prefix handling and state resolution together, which is its
own pass rather than a tail on this one.

## 2026-08-05 — Amazon EC2 Auto Scaling, and an ARN that restated a name

Both Amazon EC2 Auto Scaling ARNs carry two identifiers — one AWS assigns and
one the caller chose:
`autoScalingGroup:${GroupId}:autoScalingGroupName/${GroupFriendlyName}`. A
request supplies only the second, so neither ARN can be assembled from what the
request carries, and both are read from the simulator's own state instead. That
is the resolution Amazon RDS uses for a custom engine version, for the same
reason: the ARN the gate requests has to be the one the resource actually has.

Reading it required the simulator to have one worth reading. A group did — an
assigned identifier, generated at create and stored. A launch configuration did
not: its ARN put the *name* in the identifier slot, so it read
`launchConfiguration:<name>:launchConfigurationName/<name>` — a restatement of
the name rather than the resource's own ARN, and a policy written against the
real one matched nothing. It was also built inline in the describe response
rather than stored, which is how it stayed unnoticed. It has an assigned
identifier now, generated at create and rendered from the record.

The measurement needed the same care. Auto Scaling resolves from state, so a
probe that only fills request fields would have measured zero however well the
derivation worked — and counting table membership instead would have claimed all
44 of its operations. The probe seeds the two resources first, which is the
state any caller acting on a group is in, and the honest answer is 40 of 44.
That is why coverage reads 1,380 rather than the 1,384 membership would have
given.

## 2026-08-05 — An identifier inside a structure, and a probe that could not see it

AWS Glue was the largest single remainder, and its shape was known: the registry
and schema operations name their resource inside a structure of its own —
`RegistryId{RegistryName}`, `SchemaId{SchemaName}` — rather than at the top
level, and the JSON reader did not descend into one.

It does now, one level, and a member found at the top level still wins over one
found inside a structure: the top level is the more direct statement of what the
request names, and letting a nested member shadow it would derive from whichever
the JSON happened to carry.

The measurement had to be fixed alongside it, and that turned out to be the more
interesting half. The coverage probe sends a placeholder for every member the
model declares — and it sent a *string* for every one of them, including members
the model says are structures. So it never put an object where the API puts one,
could not exercise a derivation that reads inside a structure, and would have
counted these operations as underived while a real request derived them
perfectly well. The probe now builds an object for a structure member.

Those two changes could have flattered each other, so they were separated:
with the honest probe and the descent removed, AWS Glue is back to 55 and the
total to 1,320. The whole gain is the derivation reading nested identifiers, not
the metric loosening. Coverage is 1,340 of 1,973.

Two loose ends of the console's removed `/sim/v1` surface went with it (GitHub
issue #879). The hooks and the Go routes were already gone, but
`ui/packages/core/README.md` still advertised `useSimHealth` and `useSimSummary`
— the same trap the issue describes, relocated to the documentation — and the
Azure spec validator still carried a `/sim/v1/` skip prefix, along with the
conformance test's allowance for it, that no route could match. GitHub issue
#889 needed nothing: `CreateSecret` was fixed when the branch that reported it
was consolidated, and its regression passes.

## 2026-08-05 — AWS CloudTrail, Amazon EventBridge, and one resource published twice

Coverage reaches 1,320 of 1,973 served operations across thirteen services.

AWS CloudTrail publishes three of its four identifiers under names the API does
not use: a channel is addressed as `Channel`, an event data store as
`EventDataStore`, a dashboard as `DashboardId`. A trail is the exception, and
the prefix drop resolves it. Three aliases, each held to the vendored model.

Amazon EventBridge needed no renamings but posed a question the machinery had
not met: the reference publishes a *rule* twice. On the default event bus its
ARN is `rule/${RuleName}`; on a custom bus it is
`rule/${EventBusName}/${RuleName}`. Both end in the same variable, so both claim
the same request member, and the ambiguity rule — which exists to stop the gate
inventing an ARN — would have discarded the pair and derived nothing.

The two are not really ambiguous, though, and what settles them is not spelling
but the request: naming a custom bus means the nested ARN, naming none or naming
"default" means the flat one. So the type that cannot apply is dropped before
the derivation runs, leaving one claim on the member. A negative control
confirms it is load-bearing — forcing the wrong branch makes a rule on the
default bus derive nothing at all.

Amazon EC2 Auto Scaling's model is vendored now but its derivation is not
written: its ARNs carry an AWS-assigned group id beside the friendly name the
request supplies, so it needs the state-resolved treatment Amazon RDS's custom
engine version has rather than a field lookup. Vendoring the model is the half
that had to come first, since without one there is nothing to hold an alias
table to.

## 2026-08-05 — Two more services, and an action the reference does not list

Amazon ElastiCache and Amazon DynamoDB derive their resources now, taking the
gate to 1,262 of 1,973 served operations.

ElastiCache is the one service so far that needed nothing hand-written. Every
one of its twelve resource types is published under the parameter the API
actually sends, so its extractor is the shared derivation and a lookup, with no
alias table at all — which is what the machinery was meant to make possible.

Amazon DynamoDB retired its per-service case, and in doing so found something
the reference does not say. Most of its types are ordinary: a table stands
alone, an index nests under one, a backup and an export and an import are named
by their own ARN. But a transaction carries its table references per item, and
the AWS Service Reference lists neither `TransactWriteItems` nor
`TransactGetItems` — not with an empty resource list, but absent entirely. A
derivation driven by the reference therefore cannot reach them at all.

That surfaced as a real regression rather than an observation: moving DynamoDB
to the table-driven path made a single-table transaction fail against its own
table, because the gate had stopped deriving any resource for it and fell back
to a literal `"*"`. The behaviour is kept ahead of the reference, where it does
not depend on AWS having listed the action, and a test asserts the reference
still does not list it — so if AWS adds it, the note stops being true out loud
rather than quietly.

## 2026-08-05 — Amazon DynamoDB vector search, and a Compute member that was missing

Two gaps that had been filed rather than closed are closed.

Refreshing the vendored Amazon DynamoDB model brought `SearchVectors`, and with
it nineteen shapes describing a surface rather than a call. It is served now,
lifecycle and all: a vector index is declared with the table or added through
`UpdateTable`, reported by `DescribeTable`, and dropped the same way, carrying
the attribute holding the vector, the number of dimensions, and the distance
function to compare under.

The search reads each item's own vector attribute — the list of numbers a Put
already wrote, rather than a parallel copy that could drift from it — scores it,
and answers with the nearest TopK in order, each result carrying the score the
comparison produced. Which way a score sorts is the difference between the
nearest neighbours and the furthest: COSINE and DOT_PRODUCT rank larger-first,
EUCLIDEAN smaller-first. A zero vector has no direction, so it is refused a
cosine rather than scored 0, which would rank it as merely orthogonal — a claim
the geometry does not support. Mismatched dimensions and an unmodelled distance
function are refused rather than defaulted, because defaulting scores by a
measure the caller did not ask for and the answer still looks valid.

The end-to-end tests place their documents so the correct answer differs from
the order they were written in, which means storage order cannot produce the
result; removing the sort fails them, as a negative control confirmed. Amazon
DynamoDB is complete at 58 of 58. The installed AWS command-line client does not
yet expose the operation, so the official SDK carries the client-surface
coverage.

The Compute Engine gap was smaller and the same in kind. Discovery revision
20260729 added a standardized `resourceMetadata` beside an accelerator type, a
reservation and a future reservation, and the simulator answered without it. It
is stamped on exactly those three, carrying the canonical AIP-123 type name
derived from the resource's own kind so it cannot drift from it. Stamping it
more widely would invent a member those schemas do not declare, which is the
same defect as omitting a real one. The schema's other member, `apiVersion`, is
left absent rather than guessed: the document gives its shape by example without
stating what Compute v1 answers with.

## 2026-08-05 — Two Amazon RDS ARNs that named nothing real

Deriving Amazon RDS left two of its twenty-four resource types alone, and the
reason was not that the request withheld something. The simulator was building
both ARNs in a shape AWS does not publish: a custom engine version as
`cev:<engine>/<version>`, dropping the identifier that is its third part, and a
proxy target group as `target-group:<proxy>/<group>` where AWS publishes a
single identifier. Both named nothing real, so a resource-scoped grant written
against either was evaluated against an ARN for something else — and deriving
them then would have had the gate build one shape while the handlers built
another.

Each type gained the identifier its ARN needs. Neither is a member of AWS's own
wire shapes — checked against the vendored model before adding them — so both
are internal state that exists to build the ARN, and neither is rendered;
returning a member AWS does not have would be the same defect facing the other
way.

With the shapes right, both derive. They are the first types whose identifier no
request carries at all: a caller names a custom engine version by its engine and
version, and a target group by its proxy and group name. So the gate resolves
both through the simulator's own state and requests the ARN the resource
actually has, which is the only property that matters — an ARN that is merely
well-shaped still denies every policy written against the real one.

The coverage number did not move, and that is honest. The probe that measures it
fills every field with a placeholder and cannot create state, so these two
operations still count as underived even though the derivation works. Filling a
field with an ARN because the metric wanted one would be measuring the
measurement.

## 2026-08-05 — AWS Systems Manager, where a variable name carries meaning

AWS Systems Manager took the derivation past what a spelling table can express,
in two ways, and both were the reference telling us something rather than an
inconsistency to work around.

Four of its resource types — a maintenance window, an OpsItem, that item's
metadata and a service setting — are all published under one variable,
${ResourceId}, and the request names each of them differently: WindowId,
OpsItemId, OpsMetadataArn, SettingId. A table keyed by variable alone cannot
hold that, and worse than failing it would have made all four resolve to
whichever field the request happened to carry, so the ambiguity rule would
discard them as competing claims on one field. An alias may now be keyed
"<resourceType>.<variable>" and answers for that type alone.

A parameter is published as ${ParameterNameWithoutLeadingSlash} and named
"/db/password" in every request, so its ARN is "…:parameter/db/password". The
variable's own name states the transformation, and applying it is what stops the
gate building an ARN with an empty first path segment that matches no policy.
A parameter without a leading slash keeps its name exactly.

Four more of its types are another service's resource outright — an instance is
an Amazon EC2 ARN, a task an Amazon ECS one, a role an IAM one, and a bucket an
Amazon S3 ARN carrying neither region nor account — which the published format
already gets right and a hand-written assembler would not.

Coverage went from 1,086 to 1,170 of 1,973 served operations. The 17 that
remain name a resource by a bare identifier and a separate ResourceType member
rather than by ARN, create something that has no identifier yet, or are scoped
by a path or an operating system rather than by a resource.

## 2026-08-04 — Amazon RDS, and an alias table read off the model

Amazon RDS is the service where the reference and the API disagree about
spelling almost everywhere. The Service Reference calls a database instance
`${DbInstanceName}`, a cluster `${DbClusterInstanceName}`, a parameter group
`${ParameterGroupName}`; the API sends `DBInstanceIdentifier`,
`DBClusterIdentifier` and `DBParameterGroupName`. No rule about spelling
bridges that, so fourteen of the twenty-four resource types need an alias — the
largest of the three tables, and the one most in need of being right.

It was read off the model rather than guessed. For each resource type, the
operations that authorize against it were collected and their identifier-shaped
members ranked by how many of those operations carry them; the winner was the
alias. `DBClusterIdentifier` appears in 24 of the cluster's 43 actions,
`DBInstanceIdentifier` in 25 of the instance's 42, `DBSubnetGroupName` in 14 of
17. That is evidence rather than pattern-matching, which matters here because
the guard had already rejected two AWS Glue guesses and one of them was for a
resource type no action authorizes against at all. Every RDS alias passed the
guard first time, which is what reading them off the model buys.

The derivation itself needed one addition. RDS speaks awsQuery, which boxes a
list as `Names.member.1` where Amazon EC2's protocol flattens it to
`InstanceId.1`, so the parameter indexer now collapses both encodings and is
shared by the two.

RDS also names some resources by ARN outright, under three different parameters
— tagging sends `ResourceName`, an activity stream `ResourceArn`, a maintenance
action `ResourceIdentifier`. The gate already read an ARN a request names, but
only from a JSON body, so a query-protocol service naming one was authorized
against `"*"`. It reads the form parameters too now, taking only a value that
is actually an ARN.

Coverage went from 967 to 1,086 of 1,973 served operations. The three coverage
probes now run through the same entry point production uses rather than calling
each service's extractor directly — the numbers were unchanged by that, which is
the point of checking.

Two Amazon RDS resource types are left underived on purpose. The simulator
builds a custom engine version's ARN as `cev:<engine>/<version>`, dropping the
identifier AWS publishes as its third part, and a proxy target group's as
`target-group:<proxy>/<group>` where AWS publishes a single identifier. Every
other RDS type agrees with the published shape, including the region-less global
cluster. Deriving those two would have meant the gate building one shape while
the handlers build another, so they are recorded as the ARN defect they are
(BUG-2927) instead.

## 2026-08-04 — AWS Glue, and one derivation for any published ARN format

Amazon EC2's derivation was written against Amazon EC2. AWS Glue is the service
that shows what was actually general about it and what was not.

What was general is the idea: the Service Reference publishes an ARN format per
resource type, and that format says both what to build and what identifier to
look for. What was specific is everything else. EC2's formats all end in exactly
one identifier; Glue's nest — a table is `table/${DatabaseName}/${TableName}`
and a user-defined function carries its database too — and its root catalog
names no identifier at all, because a request authorizes against the catalog by
existing. So the filler takes a list of variables rather than a trailing one,
builds an ARN only when the request names every part, and emits a format with no
variables as the constant it is. A half-filled ARN would carry a literal
`${DatabaseName}` and match no policy, which would deny a grant that named the
real table.

The other thing EC2 did not need was a way to read a field. Glue speaks awsJson,
where an identifier is a member of the request body; EC2 speaks its own query
protocol, where it is a form parameter. That is the whole difference between
them, so it became the argument — one derivation, one lookup per service — and
the EC2 extractor is now four lines over the same core.

Glue also settled an ordering that was wrong and had not yet been caught. The
candidate spellings for an identifier ran exact, then the mechanical prefix drop,
then the service's declared renamings. On GetTable that ranked a catalog's
`${CatalogName}` dropping to `Name` above its declared `CatalogId` — and `Name`
on GetTable is the *table's* name, so the catalog and the table claimed one
field and the ambiguity rule discarded both. A declared alias is evidence, held
to the vendored model by a test; the prefix drop is a guess about spelling. The
evidence now outranks the guess.

Coverage went from 818 to 967 of 1,973 served operations. Glue's 55 remaining
are the registry and schema operations, which name their resource inside a
nested member rather than at the top level, and the data-quality operations,
which name a result rather than the ruleset they authorize against.

Two of the five aliases first written for Glue were wrong, and the guard that
holds each one to the model caught both before anything ran: one named a member
the API does not have, and one was for a resource type no Glue action authorizes
against at all. A third test was worse than wrong — it passed for a reason that
had nothing to do with what it claimed to check, because the ambiguity rule
discarded the type before the nesting rule could matter. Breaking the rule on
purpose is what exposed it; the test now names a case where nothing else can
account for the absence, and it fails with exactly the malformed ARN it exists
to prevent.

One defect came out of watching this branch's CI rather than out of the branch.
An Amazon EBS volume could report a successful detach and still be holding the
attachment a moment later, so the DeleteVolume that followed was refused as
`VolumeInUse`. Restamping the volumes an instance holds walked a listing and
wrote each row back from the copy it had listed, so a detach that landed
in between was undone. Both that walk and the one that releases a terminating
instance's volumes now decide from the value under the store's write lock, and
the listing only chooses which rows to visit. It took a slow instance
transition to be visible, which is why the hosted runners saw it once and no
local run ever did — and why the fix is pinned by two concurrency regressions
that reproduce it deterministically rather than by a green run (BUG-2926).

## 2026-08-04 — Amazon EC2 authorizes against the resource the request names

Amazon EC2 was the largest single hole in resource-scoped authorization: 515 of
the simulator's served operations declare a resource type and every one of them
was authorized against a literal `"*"`, which matches only a policy whose
Resource is itself `"*"`. Any least-privilege policy written against an EC2
resource was denied. Four parameters were read before this — VolumeId,
SnapshotId, InstanceId, NetworkInterfaceId — against the 112 resource types EC2
declares.

Transcribing 112 types was never the answer; a transcription rots the first
time AWS adds one. The Service Reference already publishes an ARN format per
type, and that format carries both halves of the problem: the shape to build
and the name of the identifier to look for. `arn:${Partition}:ec2:${Region}:${Account}:volume/${VolumeId}`
says a volume ARN is regional and account-scoped, and says the request supplies
it as VolumeId. So the generator emits the formats, and the derivation fills
them.

Filling the published format rather than assembling a path is what keeps the
irregular shapes right, and they are irregular in ways a hand-written assembler
gets wrong. An Amazon Machine Image and a snapshot carry no account. The Amazon
VPC IP Address Manager types carry no region. Five of EC2's resource types are
not EC2 ARNs at all — a certificate is an AWS Certificate Manager ARN, a role an
AWS Identity and Access Management one — and those arrive as ARNs already and
are passed through rather than wrapped.

Where the request parameter is spelled differently from the format's variable,
the difference is one of two kinds. EC2 drops the resource's own leading word
from some of them: a security group's `${SecurityGroupId}` arrives as GroupId, a
dedicated host's `${DedicatedHostId}` as HostId. That is mechanical and is
derived. The rest are genuine renamings the reference did not follow — an
endpoint service is addressed as ServiceId, a key pair by KeyName, a network
ACL's `${NaclId}` as NetworkAclId — and those are the one hand-written part,
twenty entries held to the vendored EC2 model by a test that fails on a guess, a
typo, or a rename AWS has since made. A rule that looked plausible and gained
two operations was dropped after checking what it matched: both were structures
named like identifiers, not identifiers.

A list authorizes every element. EC2 serializes a list by repeating the member's
singular name with a 1-based index — TerminateInstances takes InstanceIds and
sends InstanceId.1, InstanceId.2 — and the previous code read only the first.
Terminating three instances under a policy naming one was allowed. Members of a
nested structure are left out: a filter names what the caller is searching for,
not what it is targeting.

Coverage went from 358 to 818 of 1,973 served operations. The measurement moved
too, and had to: the table is generated for all 112 types at once, so counting
table membership would have marked all 515 EC2 operations covered when 460
actually derive. The coverage test measures EC2 against the vendored model's own
request parameters instead, which is why the number reported is 818 and not 873.
The 55 that derive nothing are the ones the request genuinely does not name — an
operation that creates its resource has no identifier for it yet, the
Disassociate and Detach family names an association rather than either end of
it, and CreateTags carries identifiers of mixed types with nothing published to
map an id back to its type. Those are recorded rather than guessed at, because a
guessed ARN denies a policy that should have been allowed and nobody would see
why.

One more joined them on review, from auditing every action that derives more
than one ARN. Two resource types can resolve to the same request parameter, and
then the identifier belongs to one of them without the request saying which:
AssociateRouteTable authorizes against an internet gateway and a virtual private
gateway and takes a single GatewayId. Building both would invent an ARN for a
gateway that does not exist and then require it to be allowed — denying a policy
that named the real one. Only three such collisions exist across the whole
surface, and one of them is answerable: RunInstances authorizes against a subnet
and a secondary subnet, and SubnetId is the subnet's published variable outright
where it reaches the secondary only through the prefix-drop rule, so the exact
spelling takes it. Where the claims are equally strong neither is derived, and
the request's other parameters still are.

Three things surfaced while verifying this branch and are fixed or filed with
it. The vendored Cloud Functions v2 Discovery document was refreshed to revision
20260730 — the local fetch kept returning 20260723 because Google's edges serve
different revisions, so the newer document came from the artifact the freshness
job captures for exactly that reason; nothing in it changed but the revision, no
method, schema or field. BUG-2924 records a VPC network left behind by a dead
simulator holding its CIDR under a name no later run looks for, which is what
made an Amazon ECS awsvpc test fail once the runs that created the orphans were
killed. BUG-2925 records the user-interface job stalling for twelve minutes with
no output on a package that builds in 398 ms; its turbo invocation lost the
daemon and the telemetry request, and each step gained its own timeout so a
recurrence names the step it stalled in — hardening, not a proven cause.

## 2026-08-03 — The eight operations that happen inside the machine

Microsoft.Compute virtual machines are complete at 29 of 29. The eight that
were left are the ones defined by what happens *in* the guest, and each now
does it.

A Custom Script extension runs its commandToExecute inside the machine and
reports the output and exit status it produced. That is the whole difference
between this and a record: a command that exits non-zero leaves the extension
Failed rather than provisioned, and the instance view carries what the command
wrote, which is what an operator reads to find out what an extension actually
did. A handler this simulator does not implement is refused rather than
reported succeeded — a caller cannot tell a handler that did nothing from one
that ran. Protected settings are accepted, stored so the handler can read them
exactly as the guest agent does, and never returned; returning them would hand
back a secret the caller entrusted to the machine.

Patch assessment reads the guest's own package manager. `apt-get --simulate
upgrade` reports what it would do without doing it, and every field comes out
of that: the package name, the version it would move to, and the archive the
version comes from — which is the only honest source for whether an update is a
security update, since apt has no Windows-style classification. The
pending-restart flag is `/var/run/reboot-required`, the signal Debian and Ubuntu
actually use. A guest with nothing upgradable reports nothing, and that is a
truthful assessment rather than an empty one waiting to be filled in.
Installation runs the same package manager under the caller's include and
exclude masks and honours the reboot setting.

Capture copies the disk the machine is running on. The guest is asked to flush
its buffers first, because a disk copied mid-write produces an image that boots
into a repair, and the copy goes into the named container through the same Blob
data plane a client reads it back from. It is refused unless the machine was
generalized — which the previous pass implemented — because an image of a
specialized machine carries its host name, its keys and its logs into every
machine created from it.

What can be asserted without nested KVM is asserted where anyone can run it:
reading a real apt plan and classifying by archive, matching package masks,
which settings the command is taken from, and every refusal — an unknown
handler, a machine that is not running, an extension with no command, a capture
of a machine that was not generalized or that names no destination. Both
decisions most likely to rot quietly were negative-controlled: breaking the
security classification and letting protected settings through each fail their
test. The paths that need a booted guest are driven by the official SDK and the
real az CLI on a capable host, asserting on the marker the command printed
rather than on the operation returning.

## 2026-08-03 — A way to run a command inside a guest

The eight Azure virtual machine operations left unserved all needed the same
thing: a way to run something inside the machine. That was staged as a
guest-side agent plus a host-side transport — vsock, or the serial console.
Building it started with reading the root filesystem Firecracker actually
publishes, and the design collapsed: it already runs OpenSSH, socket-activated
at boot, with host keys generated and a /root/.ssh directory waiting. The guest
is already reachable over its network interface — the boot wait proves exactly
that on every start.

So there is no agent and no new transport. The host authorizes itself the way an
operator does: an ed25519 key generated per machine, its public half written
into the guest's authorized_keys before the root filesystem is packed, which is
what a cloud does with the public keys in a machine's OS profile. `Exec` then
runs the command over SSH, from inside the network namespace the guest's address
lives in — the same namespace the boot wait pings from.

The mechanism turned out not to be new to this repository at all, only newly
reusable. The Firecracker arithmetic harness has always reached its guest this
way, with the same ssh-keygen call, the same authorized_keys placement and modes,
and the same client options — and it runs on real hardware in CI, which is what
establishes the approach rather than any argument here. What was missing was any
way for the simulators to do it, so every operation that needed to run something
in a guest had nothing to build on. Writing a private channel alongside that
would have invented a second mechanism where the machine already had a real one.

A command that runs and fails is a result with a non-zero exit code, not an
error. Error is reserved for not being able to run it at all, so a caller can
tell "the guest said no" from "the guest could not be reached" — a distinction
the SSH client makes for us by reporting its own failures as 255, which a
command could only return by having run first.

The half that needs no booted machine is asserted where anyone can run it:
the key is installed in the format sshd reads, at modes sshd will accept —
asserted rather than assumed, because sshd answers a group-writable
authorized_keys with a bare authentication failure — a fresh key per machine so
the host cannot reach a later machine with an earlier one's credential, and the
invocation carrying the namespace, the key, no prompt, and a terminating `--`.
Those requirements deliberately ask only for a key generator, not for Linux:
authorizing a root filesystem is portable, and only reaching the guest afterwards
needs the namespace. Gating both behind Linux would have left the portable half
untested everywhere except CI.

## 2026-08-03 — Azure virtual machines, ten more operations against the real guest

Microsoft.Compute virtual machines served 11 of 29 operations. Ten more are
served now, and they divide by what they actually needed rather than by how
much work they looked like.

Four answer off the machine's own state. `ListAvailableSizes` and
`ListByLocation` are reads, though the second is a filter that had to be a
filter — a listing that returned every machine regardless of location would
pass a test that only checks the machine it created is present, which is why
the assertion checks the other location too. `Generalize` is refused while the
machine runs, because generalizing a running system would capture it mid-write,
and once it succeeds the instance view reports `OSState/generalized`, which is
where a caller reads it back. `ConvertToManagedDisks` really rewrites the
storage profile — the `vhd` reference goes, a `managedDisk` reference arrives —
and leaves a machine already on managed disks untouched, exactly as the real
conversion does.

Six move the real guest. `Redeploy`, `Reimage`, `Reapply` and
`PerformMaintenance` mean different things on Azure's hardware but the same
thing to a Firecracker process: stop it and bring it back, leaving the machine
running. A machine that was deliberately stopped stays stopped, because these
operations restore a machine to the state it was in rather than start one.
`SimulateEviction` applies only to a Spot machine — a regular machine is never
evicted, so asking is refused — and the eviction policy decides what is left
behind, deallocated or deleted.

`RetrieveBootDiagnosticsData` returns the console output the guest really
wrote, read out of the Firecracker console log, stored into the storage account
the machine's diagnostics profile names, and served back through the same Blob
data plane a client downloads it from. It returns no console screenshot: the
guest is a serial-console machine with no framebuffer, the member is optional
in the response, and a URI pointing at an image that does not exist would be
worse than its absence.

Two members the model had been dropping are now carried. `diagnosticsProfile`
is what boot diagnostics reads, and a member the model drops reads back missing
— perpetual drift for any client that sent it, which the Terraform machine now
sends. `priority` and `evictionPolicy` are what make a machine a Spot machine;
without them every machine looked regular and eviction could not have meant
anything.

The logic is asserted in-process against a machine put straight into the store,
because booting a guest needs nested KVM and a host without it would otherwise
leave routing, state transitions, disk conversion and diagnostics resolution
untested rather than merely unbooted. The guest-moving half is driven by the
official SDK and the real `az` CLI on a capable host.

The eight operations still unserved share one blocker, and it is worth naming
precisely rather than calling them hard: an extension is a script executed
inside the guest, patch assessment reads the guest's package manager, and a
capture quiesces and copies the guest's disk. `realexec.FirecrackerVM` exposes
no exec and the guest is reachable only over its network interface, so each of
the eight served today would record something that did not happen. A guest-side
agent and a host-side transport unlock all eight at once; that is BUG-2914.

## 2026-08-03 — Packet mirroring forwards real traffic

Google Compute Engine's `packetMirrorings` was already served, and that was the
problem. All seven methods stored and returned the resource faithfully — a
policy carried `enable: TRUE`, a `collectorIlb`, a set of mirrored resources and
a filter, and read back correctly through the SDK, the CLI and the Terraform
provider — while forwarding not one frame. Nothing in the simulator could have:
the packet capture that landed with BUG-2888 supplied the read half, a packet
socket on an interface plus the five-tuple filter, and there was no send half
anywhere in the substrate.

So the send half is the work. `realexec.StartMirror` reads the mirrored
interface and transmits every frame that passes the filter out of each
collector's interface, which for the TAP backing a guest is that guest
receiving it. Frames go verbatim, headers intact and still addressed to
whoever the original packet was for, because that is what the clouds do and
what makes a collector useful — a collector consequently sees frames whose
destination is not its own, exactly as a real one must. Direction needed the
inversion stated plainly: a policy's INGRESS is traffic arriving at the
mirrored workload, which on the host side of that workload's interface is the
host *transmitting*.

The policy resolves its collector the way the resource defines it — forwarding
rule, regional backend service, instance group, instances — so pointing it at a
real internal load balancer delivers to the instances really behind it. Named
instances, every instance in a subnetwork, and every instance carrying a
network tag all select correctly, and insert, patch and delete reconcile the
running sessions, so disabling a policy stops it mirroring rather than leaving
a session behind.

Proving it took assertions at the collector rather than at the mirror, because
a session's whole claim is that a copy arrives somewhere else. Traffic
generated on one link is observed arriving on an entirely unrelated one; a
stopped mirror delivers nothing; a protocol the filter excludes reaches no
collector. The first draft of that fixture gave two false failures by
addressing the collector link, whose own IPv6 autoconfiguration and ARP chatter
is indistinguishable from a delivery at the point the assertion reads — so the
link carries no addresses and the observation is filtered to the mirrored
link's addressing, which nothing native to the collector can produce.

Two Firecracker defects surfaced while running the Terraform suite and were
fixed rather than noted. A kernel was verified and evicted only when the
root-filesystem marker was also present, and `downloadFile` returns success for
a path that already exists — so one interrupted fetch left a partial kernel
that was never re-downloaded, and every later run on that host failed with "is
not an ELF image" until someone deleted the cache by hand. And the check itself
was architecture-blind: Firecracker takes an ELF `vmlinux` on x86_64 but the
arm64 Linux `Image`, a PE file beginning `MZ`, on aarch64. Firecracker's own
published assets confirm it — the aarch64 kernel begins `4d 5a`, the x86_64 one
`7f 45 4c 46` — so every arm64 host reported a perfectly good kernel as a
corrupted download. With both fixed an arm64 host boots the kernel, and what
remains there is the boot itself rather than a format complaint that was never
the problem.

## 2026-08-03 — Resource-scoped IAM grants authorize the resources they name

The call-time IAM gate decides an Amazon Elastic Container Registry call
against the resource the request targets, which it had never derived.
`iamResourceARNsForRequest` resolves a target ARN service by service and had no
ECR case, so every ECR call fell through to the literal `"*"` — and a requested
resource of `"*"` matches only a policy whose `Resource` is itself `"*"`. A role
granting `ecr:DescribeImages` on `repository/edd-dev/*` was denied on every
repository it named. The fallback's own comment called this "the conservative
default that never over-denies"; it is the opposite, and the comment now says
which way it fails, so the next missing service reads as this same defect
rather than as a new one.

ECR names its target in the request body, so the gate reads it there:
`repositoryName` on the per-repository operations, the `repositoryNames` filter
a `DescribeRepositories` call passes, and `resourceArn` on the tagging
operations. Every named repository must be authorized, which is what AWS does
with a filtered call. Registry-wide operations — `GetAuthorizationToken`, an
unfiltered `DescribeRepositories` — name no repository and keep the
account-level `"*"` the other services use for the same case.

Repository names contain slashes and IAM's `*` spans them, so `edd-dev/*`
covers `edd-dev/golden/omnibus`. The glob matcher already behaved that way; the
regression pins it rather than assuming it, alongside the derivation for each
request shape and a repository outside the granted prefix that stays denied.

The same deployed control-plane role scopes AWS CodeBuild, Amazon CloudWatch
Logs, Amazon ECS, `iam:PassRole` and AWS WAFv2, and those statements were
denied for the same reason. Writing five more cases by hand would have repeated
the mistake in a sixth, because the thing the switch kept getting wrong was not
the extraction — it was knowing which resource a given action authorizes
against. AWS publishes that, machine-readable, at the Service Reference
endpoint, and the Smithy models the simulator already vendors do not carry it
(only Amazon ECS and AWS Lambda ship the `aws.iam#iamAction` trait). So the
Service Reference for all 33 gated services is vendored under
`specs/cloud-api/aws/service-reference/`, `scripts/gen-aws-iam-resource-types.sh`
generates the action-to-resource-type table the gate reads, a test rebuilds that
table from the vendored documents and fails on any divergence, and the freshness
gate pins each document by the published index's `modified` timestamp.

That left only the half no specification can state — pulling the identifier out
of the request — which is now written for AWS CodeBuild (including the project
inside a build id), Amazon ECS (cluster, service, task, container instance,
task set, capacity provider, and the revision `RegisterTaskDefinition` is about
to assign, which the simulator knows because it owns the counter), IAM (whose
ARNs are global and carry no region), Amazon CloudWatch Logs and AWS WAFv2.

The log-group question the bug flagged is settled by the reference rather than
by preference, and the answer is not the accommodating one: a log-group ARN
carries no trailing `:*`. Exactly four actions — `CreateLogStream`,
`DeleteLogStream`, `GetLogEvents`, `PutLogEvents` — authorize against the log
*stream*, whose ARN is the group's with `:log-stream:<name>` appended; the other
36 authorize against the group. A grant written `"${arn}"` alongside
`"${arn}:*"` therefore covers both, and one written only as `"${arn}:*"` covers
the stream writes and not the group reads — which is what real AWS does, so the
simulator does it too.

`iam:PassRole` is not an operation anyone calls; it is a second authorization
AWS runs against the role a request hands to a service. That check now runs,
carrying `iam:PassedToService` from a table generated out of every vendored
document, and the roles are found by scanning the request for role ARNs rather
than by a list of field names that would miss the next service. A caller
allowed to register a task definition but allowed to pass only one role is
denied when it passes another, which is the entire point of scoping PassRole.

Two things were fixed on the way past: `cbARN` hardcoded `us-east-1` and
`123456789012` instead of the configured region and account, and
`GetLogEvents` served an unbounded page anchored at the oldest event. The
second cost a reader real time — the service returns at most 10,000 events or
1 MB and, with `startFromHead` unset, anchors that page at the *end* of the
stream, so a default read of a busy stream showed the beginning of history and
looked like a workload producing nothing. The page is bounded now whether or
not a limit is passed.

What remains is measured rather than unknown. 358 of the 1,972 served
operations that authorize against a resource type derive it;
`TestIAMResourceDerivationCoverage` ratchets that number and prints the
per-service remainder, largest first, so the next service is a decision about
which to take rather than a rediscovery of the gap. That remainder is BUG-2909.

## 2026-08-03 — Recording real traffic, and Azure Network Watcher packet captures

BUG-2888 asked for six operations and was really asking for a mechanism: a
packet capture's entire content is the traffic that crossed an interface, so
there was nothing to serve until something could record it.

The mechanism is an `AF_PACKET` socket — the same one tcpdump uses — opened on
the interface inside the workload's own network namespace. Entering that
namespace is the delicate part, because `setns` applies to a thread rather than
a process: the socket is opened on a locked operating-system thread that is
returned to the host namespace before release, and a thread that cannot be
returned is deliberately leaked instead of handed back to the scheduler. One
leaked thread costs far less than unrelated goroutines silently running inside
a workload's namespace. The capture file is libpcap written directly, and it is
validated by `tcpdump` and `file(1)` rather than by the code that produced it.

Three things were wrong until real traffic went through it, and none of the
unit tests caught them because they had been written to match the same beliefs
as the code. A capture filtered to TCP recorded the link's ARP, because frames
with no readable five-tuple were being kept on the theory that a filter should
not remove what it could not evaluate — real tcpdump does not behave that way,
and neither does this now. A refactor let the kernel truncate frames, which
destroyed the original length the format exists to preserve. And the capability
gate demanded nftables, which a capture never uses.

All six `PacketCaptures_*` operations sit on that mechanism, taking Network
Watcher from 29 of 35 operations to 35 of 35. `Create` resolves the target
machine to the live interface carrying its traffic; `queryStatus` reconciles
against the session, so a capture that reached its own bound reports `Stopped`
with the reason instead of still claiming to run; `Stop` writes the recording
into the storage account the capture named, through the same Blob data plane a
client downloads it from. A target with no live interface is refused with the
reason, because a caller cannot distinguish a fabricated empty capture from a
real one that saw no traffic.

A container's stdout became a first-class CloudWatch ingestion alongside it.
The Amazon ECS and AWS Lambda log sinks had appended into the event slice and
stopped there, so the stream record itself never moved: an Amazon ECS service
task that had been writing for hours still reported the instant its stream was
created, ordering a service's streams by `LastEventTime` ranked the live task
as the stale one, and the log group's metric filters and embedded-metric
documents never saw a line the workload wrote — so an alarm on a running
service's error output could not fire. One ingestion helper now carries a
workload line the whole way, exactly as `PutLogEvents` carries an API caller's.

The live-streaming guard was incomplete in the same shape. BUG-872's regression
drove a one-shot `RunTask`, whose output also reaches CloudWatch through the
post-exit drain — so it passes with the live follow removed, which is the half
a service task depends on and never gets, because a service task does not exit.
The suite now drives a real Amazon ECS service and requires the container's
stdout in CloudWatch while the task is still RUNNING. Without the follow that
regression fails on a stream holding nothing but the synthetic "container
started" event, which is exactly what a deployment pinned to a build older than
that fix serves.

Google Compute Engine's `packetMirrorings` did not land with it. Capture and
mirroring share only half a mechanism: a capture records frames into a file,
while a mirroring policy continuously forwards duplicated frames to a collector
load balancer. The read half is now available; the forwarder is not, and a
policy that recorded `enable: true` while mirroring nothing would be the same
fiction packet captures were before this work. It is staged with that reason in
PLAN.md.

## 2026-08-02 — Gating specification freshness on every cloud again

`check-spec-freshness.sh` takes a cloud argument, and the `check-deps` job
passed only `gcp` — so the Amazon Web Services and Microsoft Azure halves that
BUG-2652 made a required gate had silently stopped running, and seven AWS
Smithy models plus two Azure swaggers drifted unnoticed. All three clouds are
gated again, each as its own step so a drift names the cloud that drifted and
each capturing the newer document it sampled; Azure and Google Cloud run even
when an earlier cloud failed, so one drift cannot hide another.

Refreshing the models added exactly three operations, all Amazon EC2 transit
gateway policy-table entries. They are implemented over a durable entry store
with the target route table validated on write, duplicate rule numbers
rejected, and an omitted target on modify leaving the entry pointing where it
already pointed. That refresh also exposed an existing fake:
`GetTransitGatewayPolicyTableEntries` had answered with a fixed empty list,
honest while the API modelled no way to put a rule in a table and a stub the
moment it did. It now reports what the table holds, ordered by rule number
numerically so "9" precedes "10". Amazon EC2 ratcheted from 772 to 775.

Shauth's integration into each simulator gained the regression it never had.
The relying party was mounted in all three simulators and still is — it is an
addition to each cloud's own authentication, not a replacement, and the console
federates the operator's assertion into cloud credentials through the cloud's
own primitive. What was missing was any test that a simulator actually mounts
it: `simulators/ui-auth` has twenty tests, but every one of them proves the
package in isolation, so the wiring could have regressed in any of the three
without a single test noticing and a console would simply have stopped being
able to sign anyone in. Each simulator now builds itself the way `main()` does
with Shauth configured and asserts every endpoint of the contract is routed,
that the session and federation-subject reads answer 401 anonymously rather
than 404, and that the cloud data plane still demands its own credential —
the assertion that makes "addition, not replacement" a tested property. A
negative control that unmounts the relying party fails the test on every
endpoint. The opt-in half is pinned too: with no identity-provider coordinate,
no relying party is served.

Amazon S3's three unserved operations were reclassified rather than
implemented. `WriteGetObjectResponse` is the S3 Object Lambda callback a
function makes on a per-request host; `CreateSession` and
`ListDirectoryBuckets` are S3 Express One Zone operations served from
`s3express-control` and the zonal bucket endpoints. None is addressed to the
regional surface the simulator hosts, so implementing one means hosting
another endpoint family and building the feature behind it — and neither
feature has a consumer anywhere in this repository. The reasoning, and the
condition that would justify revisiting it, now sits beside the ratchet list,
in a hand-written section of the S3 surface table that survives regeneration,
and in the conformance report, which says "not served by decision" instead of
"missing".

## 2026-08-02 — Completing Google Cloud Resource Manager, and routing before authenticating

Cloud Resource Manager was the simulator's most half-built Google service:
v3 served all 63 of its methods, but v1 served 13 of 38 and v2 — the version
`gcloud resource-manager folders` actually speaks — was not mounted at all.
All three versions are now complete: v1 at 76/76 method spellings, v2 at 24/24
over a newly vendored Discovery document, v3 unchanged at 126/126.

The bulk of v1's gap was Organization Policy: the same six methods
(`getOrgPolicy`, `setOrgPolicy`, `clearOrgPolicy`, `getEffectiveOrgPolicy`,
`listOrgPolicies`, `listAvailableOrgPolicyConstraints`) on each of project,
folder and organization. Policies are stored against the node they are set on,
and `getEffectiveOrgPolicy` resolves one by walking the hierarchy from the
organization down — a boolean policy replaces what it inherits, a list policy
replaces it unless it declares `inheritFromParent`, and `restoreDefault` at any
level discards everything above it. Etags carry the optimistic concurrency the
API documents: an empty etag writes unconditionally, a stale one is ABORTED.

The constraint catalog and the accepted set are deliberately one set. A
constraint outside the catalog is INVALID_ARGUMENT on write, and each catalog
entry's display name, description, value type and default are Google's own
from the published Organization Policy constraints reference — nothing about
them is invented. Two of them are enforced by the service they govern:
`iam.disableServiceAccountKeyCreation` and `iam.disableServiceAccountCreation`
make Identity and Access Management refuse with the exact FAILED_PRECONDITION
message and 400 status Google's own troubleshooting reference prints, so a
policy set through Cloud Resource Manager changes what another slice does
rather than being a record nobody reads.

The rest of v1 followed: `getAncestry` walks the same hierarchy leaf-first,
organizations became a real store (an organization the caller cannot see is
403, never 404, as with projects) rather than a canned response for any id,
and the liens collection is one store under both the v1 and v3 spellings —
and now enforced, since a lien exists to stop a project being deleted and had
been a record nobody consulted.

Two defects surfaced alongside it, both about answering a question before the
one that was asked. The simulator authenticated before routing, so every URI no
Google method publishes came back 401 UNAUTHENTICATED instead of 404 — an
absent endpoint and a rejected credential looked the same to a client, which is
what GitHub issue #875 had reported as a missing session endpoint. Routing now
comes first, matching Google's own frontend, verified against it:
`GET https://run.googleapis.com/nope` is 404 anonymously while
`GET .../v2/projects/…/services` is 401. The two host-addressed data planes
whose mounts match every URI sit outside that gate and authenticate themselves
once they know the request is theirs — which incidentally fixed the Compute
Engine load-balancer front end, where reaching a simulated load balancer had
required an OAuth2 token no real client sends to one.

Two more intermittent failures surfaced in this branch's CI and were fixed
rather than retried. The AWS restart harness chose the simulator's HTTP port
and its Route 53 dual-protocol port with two independent probes, each of which
released its ephemeral port before returning the number — so the operating
system could hand the second probe the port the first had just released, and
the simulator bound its Route 53 listener on its own HTTP coordinate. One
reservation helper now holds the HTTP port open across the DNS probe and fails
loudly if the two ever match. The accompanying test guards the deterministic
half of that contract and says plainly that it does not reproduce the race,
because the previous code still passes it on an idle machine.

Cloud Resource Manager v2 was vendored from revision 20260709, which is what
Google's edge served the authoring machine while the hosted runner saw
20260715. The document CI captured is now vendored verbatim; its method and
schema sets are identical, so no floor moved.

An intermittent Azure console test failure surfaced during this branch's CI
and was fixed rather than retried. The delete-error test captured the Fluent
`DialogSurface` once and used it as the `within()` scope across later
interactions; a re-render mounts a new surface and tabster's modalizer retires
the old one with `aria-hidden="true"`, which `*ByRole` queries skip. The
captured node therefore still resolved as an element while scoping every role
query inside it to nothing — the failure read "Unable to find role=alert"
against a printed DOM that plainly contained the alert. Each scoped assertion
now resolves the live dialog first, which asserts nothing weaker: a negative
control that lets a backdrop click discard the error still fails.

Every method ships with its three client surfaces. The official Go clients
cover all three API versions including `folders:search`, which has no gcloud
command; `gcloud resource-manager org-policies`, `gcloud organizations`,
`gcloud projects get-ancestors`, `gcloud resource-manager folders` and
`gcloud alpha resource-manager liens` cover the CLI; and the Terraform stack
gained `google_folder`, the three `*_organization_policy` resources and
`google_resource_manager_lien`, applied, replanned to a zero diff and
destroyed against the simulator. `google_folder` needed the provider's
`resource_manager_v3_custom_endpoint` coordinate — without it the resource
reaches real Google regardless of the v1 override.

## 2026-08-02 — Completing the Azure simulator's assessed surface

A measured survey of what remained unbuilt put the Azure storage data planes
at the bottom: Blob served 18 of its 69 documented operations, Files 13 of 51,
Queues 11 of 16, and nine vendored Microsoft.Network swaggers served nothing
at all. That surface is now complete or near it, and the measured Azure floor
moved from 1786 to 1998 operations across 91 documents.

Blob reached 69/69 with behavior rather than storage: lease state is resolved
from the clock so a finite lease genuinely expires and a pending break
completes, and it is enforced on every write path; snapshots are real
addressable byte copies; soft delete is driven by the account's own retention
configuration, taking container retention from ARM because the data-plane
contract has no such field and inventing one would have been a fidelity break;
page blobs enforce 512-byte alignment over explicitly tracked sparse ranges;
batch is real multipart re-dispatched through the simulator's own handlers;
and Blob Query returns a real Avro object-container response with its grammar
boundary enforced rather than answered with wrong rows.

Files reached 51/51, which mattered most for directories — everything below a
share root had returned 501, so nested paths were unusable and the Azure Files
volume story the Container Apps and Functions backends depend on was blocked.
Directories are now real directories on the backing store, hard and symbolic
links are real links with escape refused, snapshots are recursive copies, and
range listing reads the filesystem's own extent map. Clearing a range punches
a real hole so the extent map agrees with the client's view. Queues reached
16/16, including the visibility-timeout extension every long-running consumer
calls.

Microsoft.Network went from nothing to 116 of 123 operations across all nine
swaggers. A private endpoint writes its connection into the same object the
target resource's own surface serves, so approving on either side is what the
other reports; endpoints take real addresses from the subnet fabric;
application security group membership actually affects NSG evaluation, with an
empty group matching nothing rather than everything; the Application Gateway
is a real forwarding L7 listener that selects listeners and rules, applies
path maps, redirects and rewrites, and reports backend health from probes it
actually ran; and Network Watcher answers IP-flow-verify, next-hop,
security-group-view, topology and connectivity checks by evaluating the real
rules, routes and address space, opening real TCP connections to measure
latency. Microsoft.Subscription reached 15/15, its ownership handover
implemented against the published examples — which put the long-running
operation's polling target somewhere other than assumed, and which is the only
shape that makes the operation-get route reachable at all.

Two surfaces stayed deliberately unbuilt and are tracked rather than faked:
the Application Gateway's managed WAF rule-set catalog is data the simulator
does not carry, and packet captures have no capture path, so a session
reporting Running with no packets behind it would be fiction.

The work turned up six defects along the way, all fixed. Host-addressed data
planes — Azure storage and Key Vault, Amazon S3, Cloud Storage — served every
request outside the logging, request-id and tracing middlewares, so a whole
class of traffic was invisible in the request log; all three simulators now
separate the routing core from the observability chain. On a real-network host
every interface in an NSG-governed subnet failed to create, because the
virtual-network allow emitted a protocol the host rule compiler rejects. Both
Azure Terraform suites ran in the checked-in configuration directory and
deleted its state on entry, so two concurrent runs destroyed each other's
state mid-apply and produced what looked exactly like provider drift; both now
run in per-test workspaces, and the Docker harness stopped discarding the
caller's test selection, which is what made those collisions easy to hit.
Absolute URLs the simulator emits now honor the forwarded-proto header, so a
deployment behind its TLS gateway stops advertising http:// polling targets to
clients that can only reach it over https://. A netns name derived from an ARM
id's last ten characters collided between resources with similar suffixes.

## 2026-08-01 — The last actionable bugs: Amplify image optimization, ACA dapr, and the Azure secret-rotation/auth sweep

The three remaining locally actionable bugs closed together, and the Boy
Scout rule turned each into a wider repair.

AWS Amplify Hosting's image optimization primitive became real (BUG-2766):
requests against ImageOptimization manifest targets validate against
imageSettings — sizes, domains, remotePatterns with the real wildcard
semantics, formats, dangerouslyAllowSVG, and both minimumCacheTTL
spellings AWS's own specification uses — with the Next.js optimizer's
exact 400 strings, fetch from the app's artifacts or an allowed remote,
aspect-preserving no-upscale resize, animated and SVG passthrough rules,
Accept-driven format negotiation whose Content-Type always matches the
actual bytes (webp via a verified pure-Go encoder; AVIF never
misdeclared), strong ETags with 304 revalidation, Cache-Control flooring,
and a durable transform cache purged with its app or branch. Alongside it,
two neighbouring Amplify defects surfaced and were fixed: a
deploy-manifest.json that failed parsing had been silently ignored and the
SSR bundle served as a static site — deployments now fail with Amplify's
actual CustomerError log line (BUG-2875) — and the deploy spec's route
fallback had only ever worked for Static misses; Compute and
ImageOptimization 404s now fall through to the declared fallback without
breaking streaming, with route regexps compiled once at parse (BUG-2876).

Azure Container Apps configuration completed (BUG-2842): dapr,
identitySettings, maxInactiveRevisions, runtime, and service round-trip
at exact SDK wire spellings — exactly the member set the vendored
Microsoft.App 2025-01-01 swagger declares, verified by the runtime
spec-shape validator — and dapr
is a real runtime, not stored config: an enabled app's every replica gets
the pinned daprd sidecar sharing its network namespace, flagged with the
configured app-id/port/protocol/log settings, its stdout in the console
log table and its lifecycle tied to the replica set. A live-replica test
reads the real daprd metadata endpoint and asserts the configured
identity arrived.

The key-rotation gap (BUG-2872) closed across all seven Azure surfaces —
Storage accounts, Redis, Event Hubs, Service Bus, API Management
subscriptions, Container Registry admin credentials, and Logic Apps
access keys (whose callback-URL signatures now rotate with their key,
matching real Logic Apps) — through one durable per-resource-and-slot
generation store. Pulling that thread exposed and fixed three deeper
defects: Event Hubs had returned a single hardcoded constant as every
rule's both keys (BUG-2877), Service Bus entity deletion orphaned
entity-scoped authorization rules for same-name recreates (BUG-2878),
and — the largest — both messaging data planes accepted any
syntactically valid SharedAccessKey. Real SAS verification now guards the
AMQP CBS handshake, entity attach, management RPC, and the HTTP/ATOM
surfaces: signatures verify against the addressed rule's current
rotation-aware keys, expiry is honored, failures answer with the real
amqp:unauthorized-access condition or 401 body, every test derives real
key material via listKeys, and negative controls prove wrong, expired,
and foreign-namespace tokens are refused while a rotated-away key stops
authenticating (BUG-2879).

The 18 missing SDK/CLI coverage cells in the Azure storage data-plane
surface table were filled with real client tests, a dead-code silencer
left the RDS MySQL data plane, and a stale roadmap claim about versioned
releases was corrected.

## 2026-08-01 — Live log streaming and the cross-sim persistence sweep

GitHub issue #872 closed: all three simulator container runtimes drained a
workload's logs only after it exited, so a long-running ECS service task —
and its Cloud Run and Container Apps siblings — was log-invisible while it
ran. The runtimes now follow the Docker log stream live into the cloud
sinks, and the post-exit drain skips exactly the per-stream line counts the
live stream already delivered, so the short-lived-container EOF race stays
covered without duplicating a single line. An ECS regression proves stdout
reaches CloudWatch while the task is RUNNING and stays single after stop.

A full persistence audit of all three simulators then closed every gap
where cloud state died with the process despite `SIM_PERSIST=true` and the
same storage attached. On AWS, the EFS, EBS, and Amplify-cache bulk-data
roots moved under `SIM_DATA_DIR`, the SNS signing identity persists so
delivered `SigningCertURL`s stay resolvable, and EC2 gained the recovery
pass its ECS sibling already had — instances whose Firecracker VMs died
report `stopped` with the real `stateReason` contract. On Google Cloud, GCS
object bytes moved under the data dir, Spanner engines became real files
with incremental DDL reconciliation, Bigtable rows persist (also fixing
row resurrection after table recreation), the Compute operation registry,
Pub/Sub snapshot backlogs, and resumable-upload sessions persist, the
token-signing key survives restarts, and Cloud Run executions and Compute
instances left RUNNING by a dead process settle truthfully. On Azure, the
AWS hidden-sidecar codec was ported so stored types keep exported
`json:"-"` state (Cosmos documents became query-reachable after restart;
Files mounts, function names, Event Hubs creation times, and Entra secret
hashes survive), the Entra directory and Service Bus messages moved from
raw in-memory stores to SQLite, Cosmos consistency counters and Event Grid
key generations persist (listKeys now returns rotated keys at all), ACI
logs persist, the Azure Files content root moved under the data dir, auth
signing keys and refresh tokens survive restarts, stale InProgress ARM
operations settle Failed instead of hanging pollers, and ACR runs capture
their real docker build/push output behind the advertised log link.

Each simulator gained an end-to-end restart suite — write through real
SDKs, SIGTERM the process, relaunch on the same data dir, and read
everything back — alongside store-reopen round-trips and recovery unit
tests. The four remaining stateless key-regeneration surfaces are tracked
as BUG-2872.

The hosted run then cancelled the AWS CLI edge-delivery shard at exactly
its 15-minute limit; a latency probe proved the live-stream change adds no
per-container cost (~100ms for a short workload), and both that shard and
appdata had already been at a measured 14.4-minute edge. AWS Glue and IAM
moved into a dedicated `sim (aws cli glue-iam)` shard, returning all three
jobs to roughly eleven minutes while the coverage gate still assigns all
665 CLI tests exactly once.

## 2026-08-01 — Community-filed fidelity gaps across three sims and the deployment recipe

Two community-filed issues and two staged contract gaps closed in one pass.
GitHub issue #870: the call-time IAM gate derived a DynamoDB resource ARN only
from a top-level `TableName`, so `TransactWriteItems`/`TransactGetItems` (and
the `RequestItems`-keyed batch operations) evaluated against `*` and
resource-scoped least-privilege policies denied them. The gate now derives
the distinct per-item table ARNs and authorizes each one, mirroring AWS's
per-item evaluation; official AWS SDK and AWS CLI regressions prove the
granted table succeeds and an ungranted table denies for all four operations.

Azure Container Apps PATCH became real RFC 7396 JSON Merge Patch — one shared
generic helper merges recursively, deletes on null, and covers every modeled
field — replacing three hand-enumerated per-resource merges with three
different tag semantics. ACA DELETEs became true ARM long-running operations:
202 with an empty body and absolute `Azure-AsyncOperation`/`operationResults`
URLs from the shared ARM LRO helper (whose envelope gained `id` and whose
`operationResults` route answers 202 while in progress), a `Deleting`
provisioning state observable until completion, and 409 on writes during
deletion. The ACA-only operation store, header builder, and status route were
deleted. The still-unmodeled `Configuration` members (dapr, service, runtime,
identity settings, max inactive revisions) are tracked as BUG-2842 because
their faithful completion includes real runtime assembly.

Google Cloud Run v2 update masks stopped silently dropping fields: the
Service and RevisionTemplate models were completed to the vendored Discovery
schema, service/worker-pool/instance masks are validated against the full
mutable field set, `template.*` sub-paths merge member-wise on services and
worker pools, and an unknown or output-only path returns 400 INVALID_ARGUMENT
and changes nothing — the same contract the sim's Pub/Sub slice already
enforced.

Making `operationResults` honor ARM's real 202-with-no-body contract while
an operation runs exposed two consumers that pointed `Azure-AsyncOperation`
at that route instead of `operationStatuses`: the Event Hubs namespace and
Container Instances group creates. Both now emit the correct pairing, like
the Cosmos DB and Microsoft.Storage handlers already did (BUG-2849).

The Azure test-image and runtime-image Docker builds (and the Google Cloud
runtime-image build) gained the `--load` flag their sibling targets already
carried, so a host whose default builder is a docker-container driver gets
the image in its image store instead of only the BuildKit cache (BUG-2848).

The validation pass also caught the shared e2e harness spawning the AWS
simulator with no DNS coordinate, so its Route 53 listener bound the default
port 5353 and collided with mDNS listeners (a running browser) on developer
hosts. The harness now assigns an operating-system-selected port free on
both TCP and UDP — the repair the AWS CLI and Shauth relying-party harnesses
already carry (BUG-2847). The same-day AWS SDK patch wave was upgraded in
the AWS common/ECS/Lambda backends, the AWS SDK test module, and the e2e
module, with all upgraded modules rebuilt and retested.

GitHub issue #853: the Azure sim 502'd through the deployment proxy during
cold start, and `/auth/federation/token` was observed at ~50s. The deployment
compose recipes gained `/health` healthchecks for all four sim services,
Caddy now gates on healthy sims and converts residual proxy failures into an
explicit 503 with Retry-After on every origin, OpenID Connect discovery and
JWKS fetches are bounded by a 10-second client on a background context in
both the federation verifier and the console auth layer (whose provider
mutex previously pinned every login behind the unbounded fetch), the broker
reuses one pooled HTTP client, and the three console SPAs deduplicate
concurrent token exchanges. Azure module, shared-module, federation SDK, ACA
SDK/CLI, full GCP SDK/CLI, IAM-focused AWS SDK/CLI, three UI package suites,
265 Azure Playwright end-to-end checks, and compose/Caddyfile validation all
passed.

## 2026-07-31 — Diagnostics found a real packet-filter defect

The widened ECS Express rollout window did more than buy time: its new
diagnostic showed two replacement tasks RUNNING beside the old one for the
full two minutes, deployment IN_PROGRESS, no placement failures. The health
gate never passed because the Express-managed security group carried no
ingress permissions — and the real-VPC tier programs task security groups
into an nftables bridge filter that ends in a terminal drop. Every health
check and forwarded packet to the task's elastic network interface was
discarded, so the old task could never retire. Real ECS Express admits the
load balancer's flow to the container port; the sim's managed group now
admits TCP 443 from the world and the container port from the VPC CIDR, the
path its host-realized ALB data plane actually takes. The lifecycle test now
runs in its own VPC with a caller group that authorizes the same flow, and
its failure diagnostic reports each task's status and health and each
target's state and reason. The focused official AWS SDK Express suite
passed.

Two neighbouring defects surfaced and were fixed in the same pass. A filtered
unit-test run panicked: the delete-time drain test initialized four stores,
but its task-stop events schedule an asynchronous service reconciliation
that reads the scheduler-state, deployment-record, and alarm stores — nil
interface stores dereferenced in a background goroutine and killed the test
binary. The test now initializes what its side effects transitively read,
and the complete AWS simulator module suite passed. And the surface-table
seeder iterated filename globs in `LC_COLLATE` order, which differs between
macOS (`en_US.UTF-8`) and the hosted runners (`C.UTF-8`); identical rows came
out in different orders and the build gate reported stale tables. The script
now pins `LC_ALL=C`.

## 2026-07-31 — Specification drift and an honest rollout budget

The consolidated branch's replacement hosted run failed two checks. Google
Artifact Registry v1 had published Discovery revision 20260727 after the
revision 20260724 pin was vendored. The repository vendor script's three-probe
fetch retained the newer document; its methods, paths, and schema fields were
identical to the CI-captured drift artifact except the revision marker, and
the complete multi-probe freshness audit passed.

The hosted Amazon ECS compute shard also exhausted the Express rollout
assertion's thirty-second window. That rollout launches two replacement
tasks through serialized start and stop transitions, waits out steady-state
and target-health gating, tears down the previous task, and may be recovering
from a transient placement failure through the scheduler's bounded one to
thirty-two second retry chain — work real Amazon ECS expresses in minutes.
The window now covers the full retry budget, and a failure prints the last
observed desired, running, and pending counts, both task definitions, the
deployment rollout state and reason, and the latest service events, so a
recurrence reports its real cause instead of a bare timeout. The focused
official AWS SDK Express suite passed.

## 2026-07-31 — Hosted CI regressions became measured protocol fixes

The persistent AWS SDK harness had selected a Route 53 port by checking UDP
alone even though the simulator serves DNS on both UDP and TCP. A different
process could therefore own the matching TCP coordinate and make the child
simulator fail before an Amazon ECS restart scenario began. Package-wide and
restart harnesses now test the same wildcard port on both protocols, and a
dedicated regression repeats the actual dual bind. The real-container ECS
service-adoption restart passed with those coordinates.

AWS Glue's `ListGlossaryTerms` handler returned a convenient top-level
`GlossaryId` that the vendored Smithy model did not declare. The response now
contains only modeled `Items` and optional `NextToken` members. A signed raw
AWS JSON request and the exact AWS CLI business-context lifecycle passed the
runtime Smithy response-shape validator with no violations. The complete
service shard also showed that entity coverage assumed its DynamoDB connection
was the only table-bearing state in the shared account. The lifecycle now
locates and validates its own created entity while allowing other legitimate
tables, and its focused official AWS SDK scenario passed.

The same full shard exposed a shutdown panic that its parent process had not
treated as a failure: a Lambda event-source poller queried SQLite after the
database closed. The server now owns store-backed worker cancellation and
drain. Lambda event-source work, CloudWatch alarms, Application Auto Scaling,
and Scheduler stop before the orderly checkpoint/close boundary. A
deterministic worker-drain/reopen regression passed, and the persistent-state
SDK lifecycle now rejects a nonzero child exit before reopening SQLite.

Host disk exhaustion also left the cached Lambda Python 3.12 image pointing at
a missing overlay lower layer. Only that replaceable image was removed and
repulled; images, volumes, simulator state, and source clones were preserved.
The real managed-runtime downstream-SDK scenario passed again, and its
assertion now prints the complete Lambda failure payload instead of reducing
an actionable runtime error to `FunctionError=Unhandled`.
The exact official AWS SDK A-M shard then passed the final source state in
300.747 seconds with clean child shutdowns.

The full lint gate caught one lifecycle-construction leak: `NewServer` created
its cancellation context before persistence path and database setup could
return an error. Context creation now happens only after those fallible steps.
The exact shared-module lint and complete shared tests passed.

The repository allowed only squash merging, so the one-commit badge-refresh
PR 869 was squash-merged through GitHub into PR 868's branch. Its complete
change remained in the consolidated branch, PR 869 closed as merged, and PR
868 became the sole open pull request.

The replacement hosted run then failed before any Shauth browser assertion:
Docker Hub timed out after fifteen seconds while Compose pulled the real
PostgreSQL image. The relying-party harness now retries convergence of that
same real PostgreSQL/Ory Hydra/Shauth stack four times with bounded backoff.
Partial successful pulls and builds are reused by the next Compose convergence
attempt, while exhaustion remains an explicit failure.

The exact local rerun then exposed a second orchestration defect: the AWS
relying party inherited Route 53's fixed port 5353 and could not start while
that host DNS coordinate was occupied. Harness-owned simulators now request an
operating-system-selected Route 53 UDP/TCP coordinate, preserving the real DNS
service without a global-port collision.
ShellCheck, bash and zsh parsing, and the complete real PostgreSQL/Ory
Hydra/Shauth/Chromium relying-party matrix passed.

The Eventarc v1 Discovery document advanced from revision 20260717 to
20260723. The vendored document and provenance were refreshed; methods,
resources, and schemas were unchanged, and the complete Google Cloud
multi-probe freshness audit passed.

## 2026-07-31 — AWS VPC and Glue service models reached complete registered coverage

Amazon EC2 reached all 772 operations in its vendored service model. Regional
account VPC encryption controls persisted their mode and eight exclusion
classes, reconciled existing and newly created VPCs, and surfaced through the
Cloudscape console. VPC endpoints retained payer responsibility, and endpoint
service acceptance, rejection, and deletion established or removed the real
local PrivateLink provider connection. Official AWS SDK, AWS CLI, HashiCorp AWS
provider, and hard-restart scenarios exercised the lifecycle.

AWS Glue reached all 297 operations in its vendored service model. The final 33
operations added durable business glossaries and terms, form and asset schemas,
assets, attachments, glossary associations, iterable-form reads, data-quality
batch reads, and entity metadata/record access. Native entity reads used the
durable Data Catalog and actual Amazon S3 objects; DynamoDB connections read
real table schemas and items. IDs and client-token results survived hard
simulator replacement. The AWS console added business-glossary operations and
asset-type inventory through the public Glue APIs.

The exact service-conformance report now names missing operations for every
tracked AWS model rather than only the catalog subset. The full AWS simulator
package, official AWS SDK and AWS CLI Glue lifecycles, hard-restart matrix,
console typecheck, all 69 console package tests, production build, and focused
browser accessibility checks passed. Amazon Simple Queue Service visibility
timeout coverage also remained active in every Go test mode.

## 2026-07-30 — AWS SDK releases, local budgets, and ECS isolation stayed green

The same-day AWS SDK wave advanced Lambda from `v1.100.2` to `v1.101.0` in
the Lambda backend and official-client suite and IAM from `v1.56.2` to
`v1.57.0` in the SDK suite. The Lambda backend built and passed its package
tests, the complete official AWS SDK suite passed in 546.212 seconds, and the
repository-wide dependency, Terraform-provider, and GitHub Actions freshness
audit passed.

That complete local suite had also outgrown Go's inherited ten-minute package
deadline even though hosted CI already distributed the same exhaustive
coverage across four bounded shards. The shared Go library recipe now accepts
module-specific test flags, and the AWS SDK suite declares a 30-minute local
budget without changing test selection or hosted shard limits.

The complete official SDK run then found that generated ECS test VPCs could
reuse CIDRs owned by fixed-CIDR coverage. Test teardown also ignored the
temporary `DeleteVpc` dependency error while `StopTask` asynchronously removed
its container. ECS helper VPCs now use the reserved 10.225-249 range and retry
deletion until the workload network is released. The previously failing ECS
service lifecycle and Terraform-in-ECS cases passed with real containers.

## 2026-07-30 — Azure deletion errors remained actionable

Microsoft Azure resource-deletion dialogs could receive a Fluent UI backdrop
event immediately after Azure Resource Manager rejected the request, which
closed the surface after briefly rendering the service's real error.

Delete-error dialogs now ignore backdrop dismissal while the actionable error
is displayed. Explicit Cancel and Escape still close the dialog normally. The
Azure console typecheck, all 131 package tests, and production build passed,
including coverage that exercised the error, backdrop, and explicit-cancel
sequence.

## 2026-07-30 — Nested AWS CLI simulators gained isolated DNS listeners

The wider post-merge AWS CLI sweep launched a second process-mode simulator
while another simulator already owned the default Route 53 DNS port. The child
correctly failed its UDP bind, but the harness surfaced only a health timeout.

Nested process-mode simulators now request an operating-system-selected Route
53 UDP/TCP coordinate, just as the primary AWS CLI simulator does. This keeps
the real DNS data plane enabled without assuming a shared host port is free.
The focused process-mode AWS CLI case and the complete compute shard passed.

## 2026-07-30 — AWS orchestration and regional data fidelity closed

Load-balanced Amazon ECS deployments now continue reconciling while their
primary deployment is in progress. The scheduler retains one bounded timer per
service, re-probes real Elastic Load Balancing target health, and cancels the
timer at completion, failure, or deletion. An official AWS SDK regression
starts its real HTTP workload only after the initial steady-state probe and
proves the deployment reaches `COMPLETED` without another API request or task
transition.

Amazon ECS service discovery now follows actual service-task transitions.
Each running task registers its elastic-network-interface address, port, and
ECS attributes as an AWS Cloud Map instance; replacement, scale-in, deletion,
and hard simulator restart reconcile or deregister that instance. Failed task
launches persist their count and next-attempt time, apply bounded exponential
backoff, and drive configured deployment circuit breakers and CloudWatch
alarms to AWS-shaped failure and rollback. Restart coverage also proves that
deleting an adopted Fargate service releases its workload and network state.
Official AWS SDK and CLI scenarios passed discovery, reconciliation, both
rollback paths, and restart durability.

Amazon DynamoDB now computes item size from its stored representation,
including attribute names and recursive scalar, set, list, and map values, and
rejects writes only above the exact 400 KiB boundary. AWS Secrets Manager now
stores secrets by Region, creates independent replica records, synchronizes
primary updates, enforces primary/replica deletion rules, removes replicas,
and promotes a replica when replication stops. Official AWS SDK and CLI
coverage proved the boundary and replication lifecycle, while a SQLite
close-and-reopen regression proved regional state remained durable.

AWS Step Functions generic AWS SDK Task states expanded beyond AWS JSON
services. Repository-generated Smithy bindings now encode AWS Query, REST
JSON, and REST XML URI labels, query strings, headers, payloads, and modeled
responses through the same authenticated simulator routes. The generator
output remains compile- and conformance-tested but is excluded from the
hand-written duplicate-code heuristic.

The AWS console now lists Amazon ECS services, renders task convergence,
deployments, events, AWS Cloud Map registries, circuit-breaker and alarm
configuration, and performs desired-count updates and deletion through public
ECS APIs. Secrets Manager renders primary and replica Regions and manages
replication through public service operations. TypeScript validation, all 69
package tests, the production bundle, and all 243 Chromium tests passed. Its Playwright-owned
simulator also uses an operating-system-selected Route 53 DNS coordinate
instead of colliding with macOS's reserved multicast-DNS port.

The prior macOS Podman overlay input/output error proved volatile: restarting
the virtual machine without deleting images or volumes restored the runtime.
The complete production-shaped HashiCorp AWS provider graph then passed apply,
hard simulator replacement, refresh and external assertions, and destroy in
227 seconds.

## 2026-07-30 — Amazon ECS task roles became real workload credentials

An ECS task definition could retain `taskRoleArn`, but the launched workload
received only simulated instance-metadata credentials. AWS SDKs therefore
signed DynamoDB and other data-plane calls with an unregistered access key, and
IAM enforcement correctly rejected them. That left applications such as ECS
Dev Desktop alive but permanently unready.

The ECS task-metadata service now resolves the task definition's role, mints an
expiring temporary session, records its secret and assumed-role principal in
the Signature Version 4 credential registry, and returns the standard ECS
credential document. Linux real-VPC tasks receive
`AWS_CONTAINER_CREDENTIALS_RELATIVE_URI` at `169.254.170.2`; the
cross-platform Docker-network tier receives the same task-scoped endpoint at
its reachable host coordinate. Metadata endpoint coordinates also retain the
trailing slash required by Botocore's path construction.

A real official AWS CLI container proved the complete contract. It launched as
a Fargate task with a real IAM task role, acquired the generated credentials,
called `sts:GetCallerIdentity` through the simulator, and observed the exact
`assumed-role/<role>/<task-id>` ARN. Focused metadata and Signature Version 4
unit tests passed with the container integration.

## 2026-07-30 — Amazon ECS services became durable workload schedulers

Amazon Elastic Container Service (ECS) services stopped manufacturing
`runningCount` from desired capacity. The service scheduler launched the
declared task definition through the same real container runtime as `RunTask`,
derived service and deployment counts from durable task state, replaced
stopped tasks, honored scale-in protection, and rolled task-definition
revisions within the deployment's minimum-healthy and maximum-capacity bounds.
Desired-count updates and Application Auto Scaling converged that same
scheduler, while deletion made the service inactive before draining tasks so a
terminal callback could not launch a replacement.

Services with an Application or Network Load Balancer registered each real
task address, waited for target health before completing a rollout,
deregistered targets on stop, and preserved traffic through task replacement.
Docker Desktop exposed only the declared task ports as randomly assigned
loopback bindings; the public ECS and Elastic Load Balancing records retained
the real task elastic-network-interface address and the data plane rediscovered
the host transport from durable task labels after process replacement. ECS
Express Mode used the account's real default virtual private cloud subnets and
drove its managed task definition, desired capacity, load balancer, and target
group through the same scheduler.

Official AWS SDK tests proved placement, stop replacement, rolling updates,
scale-to-three and scale-to-zero, target health, real HTTP forwarding, and
hard-restart adoption without duplicate task identity. The AWS CLI proved
service replacement and ECS Express Mode, and production-shaped HashiCorp AWS
provider graphs completed apply, hard simulator restart, zero-change refresh,
and destroy with real service workloads. The wider audit recorded the
remaining service-discovery and failed-deployment semantics as BUG-2798 and
BUG-2799 rather than treating retained configuration as implemented behavior.

## 2026-07-30 — Dataflow and API Gateway Discovery advanced without surface drift

The replacement hosted matrix observed Dataflow v1b3 revision 20260719 after
the local freshness audit had passed and preserved the exact response as its
one-day drift artifact. That artifact replaced revision 20260715 byte-for-byte.
A structural comparison found the same 42 methods and HTTP paths and the same
1,174 schema field/type entries; only upstream descriptions and provenance
changed, so no simulator implementation or coverage floor moved.

The verification fetch then found API Gateway v1 revision 20260724 on all
three local probes. The repository vendor script retained that document and
updated its provenance. API Gateway kept the same 30 methods and HTTP paths
and the same 143 schema field/type entries, so this second same-day
publication also required no runtime change.

## 2026-07-30 — The AWS CLI appdata budget gained real margin

The updated persistence pull request passed every appdata2 command-line
interface test and its runtime Smithy ratchet, but GitHub finalized the job as
cancelled at its exact 15-minute limit. The test process alone consumed 791
seconds, after setup and a clean simulator build consumed the remaining
headroom.

Hosted top-level timings measured RDS at 199 seconds, SSM at 133, S3 at 108,
Route 53 at 67, and the remaining services at 216 seconds. The matrix now keeps
RDS, Route 53, S3, and the restart-persistence case in appdata2 and moves
Scheduler, Secrets Manager, Step Functions, SNS, SQS, SSM, STS, and WAFv2 to
appdata3. The repository coverage gate still requires every AWS CLI test to
match exactly one shard.

The same production audit exposed a distinct P1 rather than hiding it behind
successful Terraform control-plane convergence: Amazon ECS services still
report desired and running capacity without launching their declared
task-definition containers. BUG-2794 owns durable service scheduling,
replacement, load-balancer registration and health, and stop/restart behavior
required for ECS Dev Desktop to become a functional deployed app.

## 2026-07-30 — Legacy Amazon ECS state stopped blocking simulator upgrades

The first production upgrade to durable workload recovery encountered an
Amazon ECS task persisted by an older release. Its control-plane row still
claimed `RUNNING`, but the prior runtime had not left a state-scoped workload
container that the new process could adopt. Recovery treated that mismatch as
a fatal simulator error, so one stale task prevented every AWS API from
starting and the deployment correctly rolled back.

Amazon ECS now reconciles exactly that zero-container case to `STOPPED`, with
`EssentialContainerExited`, a control-plane restart reason, an unknown exit
code, adjusted container-instance counts, managed-resource cleanup, and
network teardown. It continues restoring subsequent tasks. Container-runtime
discovery failures, missing task definitions, malformed identity labels, and
adoption failures remain fail-loud startup errors. Focused coverage proves
multiple legacy tasks reconcile in one pass and a real discovery error leaves
the task untouched while aborting recovery.

## 2026-07-30 — Persistence stayed private where AWS response models required it

AWS Glue database tags had been emitted inside `GetDatabase`, a member the
Smithy response model does not define. Simply hiding the field would previously
have discarded it from SQLite. The persistent envelope retained that
internal tag state while AWS Glue exposed it only through `GetTags`. Official
SDK and Terraform fixtures created tagged databases, and the hard-restart
matrix proved the database and tags survived together.

An HTTP-only AWS Cloud Map service had also gained
`HealthCheckCustomConfig` when its caller omitted the optional configuration.
The simulator retained custom health only when the public create request
supplied it, so updating an unconfigured instance returned
`CustomHealthNotFound` as AWS specifies.

The durable Lambda restart scenario stopped using a container-to-host callback
to recover its checkpoint coordinates. The managed Node.js function logged
them to Amazon CloudWatch Logs, the official AWS SDK read those logs and
checkpointed the execution, the simulator was hard-replaced against the same
state directory, and callback completion resumed the original execution. The
complete SDK services A–M shard passed in 230.8 seconds with no Smithy
violations.

The AWS CLI Elastic Load Balancing fixtures imported real certificate and key
material through AWS Certificate Manager and selected isolated listener ports,
matching the SDK fixtures and the live TLS data plane. Nested simulator
processes received an isolated Route 53 DNS coordinate. The focused HTTPS and
Network Load Balancing cases, complete SDK compute shard, and complete CLI
edge-delivery shard passed.

The macOS Terraform container wrapper created, attached, and removed its exact
test container explicitly, propagated the real failure status, removed
anonymous volumes, and mounted runtime Smithy reports back to the host. It
therefore exposed the local Podman machine's existing overlay input/output
error instead of leaking a created container or hiding the failure; that
operator-owned storage corruption remained recorded as BUG-2791 while hosted
Terraform validation stayed mandatory.

## 2026-07-30 — The Google Cloud client graph advanced to gRPC 1.83

The pre-push publication audit found gRPC 1.83.0 after the earlier validation
had passed. Both Cloud Run backends, the shared Google Cloud backend, the
Google Cloud simulator, and its official SDK-test module upgraded together
with their selected current graph. Every affected module and the authenticated
freshness audit passed.

The complete official Google Cloud SDK run exposed a real-container fixture
defect rather than a wire regression. Its temporary registry used one
`docker run --rm` operation, so Podman waited for automatic removal after a
start failure, hid the provider error behind the test deadline, and retained
anonymous volumes. The fixture now creates and starts the real registry in
separate bounded operations and removes both its container and volume. The
focused Cloud Build build-and-push scenario passed in 8.8 seconds, and the
complete official Google Cloud SDK suite passed in 36 seconds.

## 2026-07-30 — Successful Azure validation stopped emitting failure annotations

The Microsoft Azure workload-dispatch invariant logged its two justified
`os/exec` exceptions with Go test file-and-line prefixes. GitHub's Go problem
matcher interpreted those informational lines as failure annotations even
though the test and required check passed.

The exception reasons now live beside their entries as reviewable source
comments, and the scan skips them without runtime logging. The invariant still
fails on every unlisted use while a successful run stays annotation-free.

## 2026-07-30 — Simulator jobs stopped publishing mutated guest caches

Simulator SDK, CLI, and Terraform matrix jobs had restored the shared
Firecracker seed and then attempted to save the guest filesystem after real
workloads made system files root-only. Post-job `tar` therefore emitted
permission errors while archiving a cache that should never contain test
mutations.

Those matrix jobs now use the restore-only cache action. The dedicated
Firecracker job remains the sole publisher of the immutable seed, so simulator
jobs consume the acceleration without trying to archive their mutated guest
state.

## 2026-07-30 — DynamoDB auxiliary table state became durable

DynamoDB TTL, point-in-time recovery, and tags had been attached to table
response records as fields excluded from JSON. The in-memory store retained
them, but a persistent simulator serialized every update and immediately lost
the values, leaving HashiCorp Terraform waiting for TTL to become enabled.

The three settings now share a dedicated SQLite-backed table-settings record,
remain outside `DescribeTable`, survive a state-store close and reopen, feed
IAM resource-tag conditions, and are deleted with their owning table. The
production-shaped AWS provider fixture declares TTL, point-in-time recovery,
and tags so its apply and refresh permanently exercise every waiter.

## 2026-07-30 — Cloud SQL Admin revision 20260722 was implemented

The pull-request specification gate captured exact newer Cloud SQL Admin v1 and
v1beta4 Discovery documents. Their 75 public methods and paths were unchanged,
while the instance, on-premises configuration, and user schemas added
`databaseCenterIntegrationEnabled`, output-only `dmsManaged`, and top-level
`serverRoles`.

The simulator now persists the requested Database Center setting and SQL
Server roles and truthfully reports `dmsManaged=false` for an on-premises
source that this simulator does not attach to Database Migration Service.
Authenticated public-route coverage round-tripped all three fields, and the
exact compressed documents and provenance moved to revision 20260722.

## 2026-07-30 — Cloud state survived hard simulator replacement

The AWS simulator's durable store retained runtime configuration that public
JSON intentionally omitted, monotonic sequence state, derived Amazon ECS
revisions, listener coordinates, accepted asynchronous Lambda invocations, and
Step Functions external-task checkpoints. Startup rebound real Network Load
Balancing and Amazon RDS data-plane listeners and adopted or resumed
state-directory-owned Amazon ECS, AWS Batch, CodeBuild, Amplify, Lambda,
scheduler, and autoscaling work. Step Functions reattached to the original
Amazon ECS or CodeBuild task rather than duplicating it, while Lambda preserved
AWS's at-least-once asynchronous delivery and destination records.

The official AWS SDK hard-restart matrix passed control-plane, live-workload,
orchestration, asynchronous delivery, Network Load Balancing, and Amazon RDS
native-endpoint scenarios. The official AWS CLI retained service resources and
CloudWatch Logs sequence ordering. The production-shaped HashiCorp AWS provider
completed apply, hard replacement on the same data directory, a zero-change
refresh plan, and destroy. The deployment recipes enabled persistent data
directories backed by named volumes for the AWS, Google Cloud, and Microsoft
Azure simulators.

The same audit made Elastic Load Balancing listener creation and modification
transactional when a real TCP or TLS bind failed, completed live AWS WAF
statement evaluation, and expanded the AWS Batch Cloudscape page to real jobs,
definitions, terminal detail, polling, and termination through the standard
AWS APIs. Generated service-surface catalogs, AWS CLI shard coverage, and the
shared Google Cloud and Microsoft Azure dependency graphs were refreshed with
the implementation.

## 2026-07-30 — Client-module downloads became retry-protected

Google Cloud and Microsoft Azure SDK/CLI jobs pre-fetched their separate
official-client modules through the existing bounded module-proxy retry helper.
A transient proxy reset therefore failed or retried during the explicit
dependency phase instead of bypassing retry inside `go test`.

## 2026-07-30 — Common cloud-backend module graphs were reconciled

Go 1.26 module loading found that the Microsoft Azure and Google Cloud common
backends still selected `go-isatty` 0.0.24 while their module files recorded
0.0.22. Both graphs now record the selected transitive release, and their
focused GolangCI and unit suites passed.

## 2026-07-30 — Terraform-in-ECS failures became bounded and diagnosable

The Step Functions integration kept HashiCorp Terraform 1.15.8 and AWS
provider 6.50.0, while packaging the provider into the workload image through
an ahead-of-time filesystem mirror. The private-subnet Amazon ECS task now
performs one offline provider initialization and one fail-loud apply without
depending on undeclared internet egress.

The Amazon ECS task now sends its complete output to Amazon CloudWatch Logs.
The test requires the successful apply message and, on a terminal workflow
failure, reports the task output immediately instead of polling the failed
execution for five minutes. The focused real-container execution and exact
official AWS SDK N-Z shard passed.

## 2026-07-29 — Filesystem staging tests became privilege-independent

Core filesystem-driver tests stopped assuming a path below `/usr/local` was
unwritable. Each staging case now targets a child beneath a real regular file,
which makes direct directory creation fail consistently on local and hosted
runners regardless of user privilege.

## 2026-07-29 — Dependency pins caught up with coordinated releases

The authenticated pre-push freshness gate detected a coordinated AWS SDK patch
wave and a new Google Cloud Spanner client published after the branch began.
The repository-owned per-module upgrade fan-out refreshed every affected direct
pin and its resolved transitive graph.

## 2026-07-29 — AWS Key Management Service policies became durable state

Customer-managed AWS Key Management Service policies survived the simulator's
SQLite serialization and process restart. The stored key record retained the
exact JSON policy accepted by `CreateKey` and `PutKeyPolicy`, so
`GetKeyPolicy` returned the requested document after a subsequent durable-store
read instead of silently substituting the default root policy.

A focused regression closed and reopened the real simulator state database
before asserting the policy. The production-shaped HashiCorp AWS provider graph
also created a policy-bearing key, making the provider's create-time policy
waiter an integration guard for the persistent read-back path.

## 2026-07-29 — Cloud data planes passed external provider ratchets

Google Cloud Spanner moved from an administrative projection to a transactional
cloud slice. Official REST and gRPC clients executed SQL, DML, batch DML,
key-set reads, mutations, begin/commit/rollback, partitioned work, and batch
writes against real SQLite transactions. DDL validation became strict,
composite primary keys preserved range semantics, gcloud executed SQL and
partitioned DML, and the official HashiCorp Google provider completed
instance/database/DDL apply, a zero-change plan, and destroy. The served
Discovery floor advanced from 58 to 82 of 198 methods.

AWS Step Functions launched the official HashiCorp Terraform image in a
synchronous Amazon ECS task. The process used only `AWS_ENDPOINT_URL` and
ordinary AWS credentials, applied a tagged Amazon SQS queue back into the
simulator, and an independent official AWS SDK client read the created queue
and tags. This made the infrastructure-as-code-in-a-workload review claim an
executable test rather than an assumption.

AWS Amplify retained release build ZIPs, end-to-end test artifacts,
configuration URLs, retry lineage, build phases, and cleanup through the public
job and artifact APIs. AWS WAF association updated the Amplify app, protected
the hosted data plane with default and IP-set actions, and returned observed
traffic from `GetSampledRequests`; WebACL capacity came from the real recursive
statement calculation. Repeated AWS Certificate Manager requests for one
normalized domain reused a stable account-scoped DNS validation value.

The independent ecs-dev-desktop Terraform module exercised the standard global
AWS endpoint without simulator-aware module branches. It applied 178 resources,
passed its own shared-infrastructure assertions, produced a zero-change
follow-up plan, emitted no runtime Smithy violations, destroyed all 178
resources, and retained a clean working tree. That run exposed and fixed API
Gateway v2 defaults, update fields and stage tags, plus durable AWS Lambda
reserved concurrency returned by `GetFunction`.

The same-day dependency ratchet upgraded Google Cloud Spanner to v1.94.0 and
the affected AWS SDK graph to v1.43.2 with current service releases. The
complete official Google Cloud and AWS SDK suites passed, all five
production-shaped HashiCorp AWS provider packages passed, and the authenticated
freshness audit reported no drift.

The Google Terraform harness stopped treating absent real-execution
capabilities as a passing skip. macOS selected the privileged Linux harness,
Buildx loaded its output into the runtime, and the shared image contained
Firecracker v1.15.1 plus `unsquashfs` 4.5.1. Podman's macOS virtual machine
still exposed no nested `/dev/kvm`; that external host boundary, occupied
macOS load-balancer listener ports, AWS Amplify Hosting image optimization, and
the remaining AWS WAF statement evaluator were retained explicitly in
BUGS.md.

## 2026-07-28 — AWS Lambda and AWS Step Functions became complete executable cloud slices

AWS Lambda implemented every one of the 85 operations in its vendored Smithy
service model. ZIP archives and container images executed through the AWS
Lambda Runtime API rather than returning canned invocation results. Layers,
versions, aliases, function URLs, reserved and provisioned concurrency, capacity
providers, code-signing configuration, response streaming, and durable
executions retained real lifecycle state, validation, pagination, callbacks,
timeouts, and histories.

AWS Step Functions implemented all 37 operations in its vendored model.
State-machine definitions executed JSONPath and JSONata expressions across
Pass, Task, Choice, Wait, Succeed, Fail, Parallel, inline and distributed Map,
activities, callbacks, retries, nested workflows, and AWS Lambda tasks.
Published versions and aliases captured immutable definitions, executions
captured immutable snapshots, redrive created service-shaped continuation
history, and every history page retained the corresponding event payload.

Official AWS SDK, AWS CLI, and Terraform clients drove the public surfaces.
The Terraform stack created Lambda versions, aliases, and function URLs plus
Step Functions state-machine versions and aliases. The CLI covered the expanded
control planes and runtime behavior, including the previously exempt
ListStateMachineVersions operation. Its harness installed AWS's official
architecture-specific Session Manager plugin when necessary and removed all
provisioned tools after the suite. The Step Functions Lambda-task SDK flow used
the suite's prebuilt real Runtime API image, so its history assertion measured
the integration rather than a clean runner's unrelated managed-runtime image
download; separate ZIP and live-AWS differentials retained managed-runtime
coverage. The now-completing test exposed that history data details had used
DescribeExecution's `included` member; all history events now used the Smithy
model's `truncated:false`, including when execution payloads were omitted.

Selected flows also ran against short-lived live AWS resources. The differential
covered Step Functions validation, execution history, Lambda task history,
control-plane lifecycle, nested workflows, distributed Map, and Lambda ZIP,
layer, version, and alias behavior. The same tests targeted the simulator by
changing only endpoint and credential coordinates, and every temporary state
machine, function, layer, role, and policy was removed afterward.

The AWS console exposed Lambda overview, code, test, logs, configuration,
layers, environment variables, concurrency, versions, aliases, URLs, and tags.
The Step Functions console exposed graph and definition authoring, execution
input, event history, input/output inspection, publishing, aliases, tags, and
redrive. The production bundle passed 229 Chromium package tests, while the
authenticated Shauth/Ory Hydra/PostgreSQL browser matrix used federated AWS
credentials to create a state machine, start it, and inspect its graph and
execution history through the real APIs.

The conformance harness stopped launching runtime evaluator goroutines for
route-only simulator builds. Production builds retained Amazon CloudWatch alarm
and Application Auto Scaling evaluation, while repeated in-process
introspection no longer rebound stores underneath old goroutines. The focused
race suite and the complete AWS simulator package passed with this lifecycle
separation.

Clean Linux validation exposed that temporary AWS Lambda deployment-package and
layer roots retained `0700`, although managed runtimes correctly ran as
`sbx_user1051`. Docker Desktop had hidden the mismatch; Linux preserved it and
made `/var/task` untraversable. Both mounted roots now carried Lambda's
sandbox-readable filesystem permissions while their container mounts stayed
read-only. The ordinary ZIP invocation, durable execution, durable callback
replay, complete A–M AWS SDK shard, and Smithy response-shape ratchet passed.

The dependency freshness job retained authentication across both of its real
network-backed shell portability passes. Its GitHub authorization option moved
from a scalar expansion that only Bash split correctly to a shell-portable
argument array, so Zsh no longer converted a present token into unauthenticated
GitHub API requests after Bash had consumed the public rate-limit window.

## 2026-07-27 — AWS Lambda `VpcConfig` became a runtime network

AWS Lambda had described Hyperplane elastic network interfaces on the control
plane while launching every image function on Docker's default bridge. A
function therefore reported configured subnets and security groups that its
workload never used. `CreateFunction` and `UpdateFunctionConfiguration` now
validated that every subnet existed in one Amazon Virtual Private Cloud (VPC)
and every security group belonged to that VPC before allocating addresses.

Each VPC-configured invocation leased an address from its configured subnets.
On a Linux real-execution host, a pause container held the invocation network
namespace, a VPC veth carried the leased address, nftables enforced the
configured security groups and route-driven egress, and a unique link-local
destination DNATed the AWS Lambda Runtime API back to its per-invocation
listener. Runtime API DNAT tables, veths, pause containers, and leases were
removed when the invocation ended. Portable hosts represented the same VPC,
subnet address, and identifiers through the container engine's VPC network;
the public cloud contract stayed identical and only the local execution
substrate differed.

The official AWS SDK test built a VPC, subnet, and security group, launched an
Amazon ECS Fargate task serving HTTP on its private `awsvpc` address, then
invoked a Lambda image whose handler reached that address. AWS CLI coverage
created and invoked a VPC-configured function, and the production-shaped
Terraform stack attached its Lambda function to the same VPC it provisioned.
The complete SDK and CLI Lambda suites passed, and Terraform completed a full
apply and destroy.

Validation exposed two adjacent defects and closed them. Amazon Cloud Map had
round-tripped the deprecated caller-supplied custom-health failure threshold;
it now reported AWS's fixed value of `1` consistently through Create, Get, and
List. The Terraform fixture removed that deprecated field, moved DynamoDB's
global secondary index to `key_schema`, and configured its temporary state
through the local backend instead of the deprecated `-state` command flag, so
`terraform validate` completed without warnings.

The AWS SDK, CLI, and Terraform harnesses had also killed the simulator
directly after tests, bypassing its workload cleanup. Normal completion now
sent the simulator's termination signal, waited for cleanup, and used forced
termination only after a bounded grace period. The Terraform VPC moved away
from Podman's default `10.88.0.0/16` bridge. The passing production-shaped run
left no Lambda, Amazon ECS, or simulator VPC artifacts on the container host.

The pre-push freshness gate found a newly published
`github.com/docker/go-connections` patch release. The Docker backend and the
AWS, Google Cloud, and Azure simulator shared modules upgraded to v0.8.1 in the
same branch; the Docker backend's standardized upgrade also advanced its
indirect `github.com/mattn/go-isatty` dependency to v0.0.24. All four modules
passed their complete tests.

The first CI fan-out caught what the local workspace had hidden: each root
simulator module still selected the older indirect dependency when built with
`GOWORK=off`, so the AWS shards failed on missing v0.8.1 sums. The AWS, Google
Cloud, and Azure root modules were each tidied and tested independently with
the exact `GOWORK=off CGO_ENABLED=0 -tags noui` command CI used, and the AWS
make build passed.

Running those standalone suites concurrently exposed an Azure DNS startup race.
The server had asked the kernel for a UDP port and then assumed the same number
was free in the independent TCP port namespace. Dynamic startup now closed the
partial socket pair and retried a bounded 16 times until both real listeners
shared a port; an explicitly configured port still failed immediately. The DNS
tests passed 100 repetitions after the repair.

The first Linux run of the new Lambda-to-Amazon-ECS integration exposed that
the bridge security-group filter dropped ARP as well as disallowed IP packets.
The target had been reachable in older host tests only because they populated
the neighbor cache before installing the filter. Security-group enforcement
now permits ARP before applying IP policy, and the host regression flushes the
neighbor entry before proving permitted traffic. A fresh VPC-configured Lambda
invocation then reached the Amazon ECS task's private elastic-network-interface
address on Linux.

The wider Lambda suite exposed a second coordinate defect specific to Linux
inside a Podman machine: `host.docker.internal` named the outer macOS host, not
the Linux VM namespace where the Runtime API listener ran. The shared runtime
layer now reads the default bridge's IPv4 gateway from Docker or Podman, Linux
callbacks use that exact address, and launch fails clearly if the required
coordinate is absent. Ordinary and VPC-configured Lambda invocations passed
their complete SDK suite on Linux, and the macOS desktop path remained green.

The exact AWS services shard also exercised Amazon Amplify on an
SELinux-enforcing host. Its compute container could not read the deployed
bundle and its build container could not write the real artifact workspace
because neither bind carried a relabel. Compute now mounts the bundle
read-only with a shared label, while builds mount their workspace writable
with the same label. Both end-to-end SDK flows passed on enforcing Linux.
That audit also recorded BUG-2690: build-shaped jobs without a clonable HTTP
source and explicit build specification still reported a synthetic success
instead of performing real source/build resolution or returning an AWS error.

The final freshness pass upgraded the standalone AWS SDK graph to
`github.com/aws/smithy-go` v1.27.5. Running the whole SDK matrix then exposed a
runner failure that the DynamoDB Local differential harness had hidden:
Podman's overlay filesystem returned an input/output error while mounting the
oracle container, but the unbounded `docker run` call waited until the package's
15-minute timeout and lost the useful container state. The harness now bounded
image inspection, pull, launch, state inspection, and cleanup, gave each oracle
container a deterministic run-local name, and reported the real failed state
before cleaning it up. After the local engine remounted cleanly, the focused
oracle and all four non-overlapping AWS SDK shards passed.

Publishing the merged revision exposed a GitHub Container Registry retention
edge. The unchanged Admin ARM64 image had the old and current release tags on
one package version; deleting the version because the old tag was obsolete also
deleted the current tag. The selector now deletes a version only when none of
its tags belongs to the retained release set, and its contract fixture includes
that shared-version shape. Every native architecture build also stamps the full
source revision into its OCI config, making each future release digest distinct
even when its application bytes are unchanged.

The refreshed freshness gate then found AWS Glue SDK v1.150.0. The standalone
AWS SDK module upgraded from v1.149.0, and its complete real-simulator suite
passed with the new client model.

That replacement run exposed a workflow-budget defect after every A–M SDK test
had passed. The success path invoked `du -d 2 /` to report the runner's largest
filesystem consumers; the recursive scan spent more than nine minutes walking
the hosted image and crossed the 15-minute job limit. Successful shards now
report only constant-time filesystem, log, and Docker summaries. The detailed
consumer diagnostic runs only when the watched disk threshold trips, covers the
repository workspace, runner temporary directory, and Firecracker workspace,
and bounds each scan to five seconds. The workflow-budget gate gained a fixture
that rejects a recursive whole-volume scan.

## 2026-07-27 — Simulator conformance became a measurement, and the defects it had been hiding were fixed

The three conformance ratchets counted coverage the simulators did not have.
Google Cloud treated a documented method as covered whenever any mounted route
pattern shape-matched its path, so `{+name}` template spellings and colon-verb
fan-in handlers inflated a floor with methods the simulator answered `404` for.
Microsoft Azure did the same by shape and worse: `storage-dataplane-blob` scored
60 of 69 while the simulator had no mux routes for it at all — the Blob plane is
host-addressed, so its "covered" operations were shape matches against Azure
Cosmos DB, the OCI registry, and Azure Container Registry. Amazon Web Services
credited every versioned service with the entire unversioned bucket of its query
router; those 994 actions belong to four services, so 32 vendored models were
credited with operations they never register, which is how AWS Certificate
Manager's missing tagging stayed hidden.

All three now measure. Google Cloud and Azure probe the running simulator: each
documented method is rendered into a concrete URI from the specification's own
patterns and sent through the same handler chain `main()` serves, and a handler
must actually answer for the method to count. A Go mux miss, a method mismatch,
or a structured "method unknown" is uncovered; a structured `404` for a resource
that does not exist is covered, because a handler ran and answered. Write probes
carry deliberately malformed bodies, so no probe can mutate simulator or host
state. Amazon Web Services replays each legacy query registrar against a fresh
router to record what it actually mounted, credits an action no registrar claims
to nobody and a contested action to neither, and its
`TestQueryActionsExistInSmithyModels` tightened from "the action exists in some
query-protocol model" to "in the model of the service that registers it". Each
cloud has a soundness test that fails if the probe stops authenticating or an
unmounted URI stops reading as a miss — without one, a broken token would credit
an entire surface to a middleware rejection and the gate would measure nothing.

The numbers moved in both directions, which is the point. Google Cloud fell from
4249 to 4054 of 5397 documented methods and Spanner from 198 to 58, its entire
REST session data plane having been phantom; Azure's Blob data plane fell from 60
to 17 of 69. Azure rose overall to 1878 of 2612, because probing at the correct
host coordinate revealed real Azure Key Vault coverage the matcher could not see.
Every Amazon Web Services floor was unchanged, which is the meaningful result
there — honest attribution did not move them.

Probing exposed handlers that answered resource semantics for methods they do not
serve, hiding the gap and in one case inventing data: Firestore created a document
in a collection named after the method for `listCollectionIds`, `partitionQuery`
and `runAggregationQuery`. Secret Manager, Cloud Resource Manager, IAM, Cloud KMS
and Spanner answered "not found" or "permission denied" for unrouted custom
methods, as did the Cloud Resource Manager v3 projects fan-in, which resolved the
project before the verb. They now resolve the method before the resource, the way
Google's API frontend does, and answer `404 "Method not found."` An Amazon RDS
client calling `DescribeAccountAttributes` had been falling through to Amazon EC2's
handler and receiving EC2-shaped XML; Amazon RDS now serves it with all 18
documented quotas and every used value counted from real simulator state.

The Azure Storage data planes stopped answering with a sibling handler. An
unrecognised `comp` or `restype` had fallen through, so Set Blob Tier answered
`201 Created` having created a blob — which also meant three floors were an upper
bound rather than a measurement. Removing the fall-through uncovered three further
defects it had masked: `?comp=range` landed on Create File, so the azfile SDK's
`Create` plus `UploadBuffer` only worked by accident; Set Queue Metadata fell to
Create Queue, which answered `204` and silently dropped the metadata; and ranged
reads were missing entirely, which is why `az storage blob download` failed on an
absent `Content-Range`. Azure Files, Set Queue Metadata and ranged reads were
implemented; the rest declare a `501` gap in the Azure Storage XML envelope.

The Azure Files data plane now writes where a container reads. Two Files
implementations had existed, and the disk-backed one — the one persisting under
the directory an Azure Container Apps Job or App bind-mounts for
`Volume{StorageType: AzureFile}` — was unreachable, so a workload mounting a share
saw none of the bytes a data-plane client wrote and the share directory was never
created at all. There is now one implementation, resolving every file through the
same host path the executor binds, with the filesystem as the single source of
truth for existence, size, bytes and listings. The Azure Storage static website
(`.web.`) and Data Lake Storage Gen2 (`.dfs.`) planes, which had answered a bare
`200 OK` with an empty body to every request, declare their gap, and the dead
dispatcher behind them was deleted.

Amazon ECS honours the task definition's `networkMode`. It had been ignored at
launch: every task received an elastic network interface attachment regardless of
mode, `networkConfiguration` was accepted and ignored for non-`awsvpc` modes, an
`awsvpc` task without it was accepted and run on Docker's default bridge, and an
unresolvable subnet or VPC silently degraded to that same default bridge. Now
`awsvpc` requires `networkConfiguration` and is rejected without it,
`networkConfiguration` is rejected for other modes, the elastic network interface
appears only for `awsvpc`, each of the four modes lands on its own fabric, and an
unresolvable VPC is an error rather than a silent fallback.

Four reported issues closed, three of which were mis-diagnosed by their reporters
and were established as such empirically rather than implemented as asked. The
Google Cloud simulator's `401` on service-account key issuance was the bearer
middleware answering an unauthenticated request, not a missing endpoint; the real
blocker was the metadata server's directory listings, and gcloud's Application
Default Credentials path now works against the simulator with no static-token
escape hatch. The Azure simulator's bare origin returned `404` because Azure
Cosmos DB owns the API root for the azcosmos SDK's global endpoint manager, as
Amazon S3 does on Amazon Web Services; Cosmos now delegates the requests it
declines, keeping the root for its own clients while a browser reaches the
console. The Azure `az login` failure came from real Microsoft Entra rejecting the
reporter's authority — MSAL sends instance discovery only to
`login.microsoftonline.com` and skips it entirely for a trusted host — so the
simulator gained the instance-discovery endpoint for completeness, while the
coordinate that actually makes `az login` work, the AD FS authority form, was
documented and pinned by a regression test. The Amazon ECS container-networking
failure on minimal guest kernels is not avoidable by moving tasks off the default
bridge: Docker programs its direct-access-filtering `raw` rule for every endpoint
on any bridge-driver network, so the simulator now fails with a message naming
`iptable_raw` and `CONFIG_IP_NF_RAW`, and the guest-kernel requirement is
documented.

## 2026-07-26 — Refreshed every vendored cloud specification, and fixed the check that hid the drift

The freshness check reported every vendored specification as drifted, which is
what prompted this pass — but the check itself was the larger defect. The fetch
scripts recorded a specification's pin as the *branch tip*, while the check
compared that pin against the newest commit that touched *that file's path*. For
a stable specification those are never the same commit, so every freshly
vendored file reported drift the moment it was vendored and genuine staleness
was indistinguishable from bookkeeping noise: 117 of 117 Azure rows read drift
while 116 of those files were already byte-current. Both fetch scripts now
default to the commit that last touched the file's own upstream path, so a pin
means what the check tests.

With that understood, every specification was refreshed: 38 AWS Smithy models
(21 changed content), 29 Google Cloud Discovery documents (several a month or
more behind), and 118 Azure REST specifications. AWS and Azure now report no
drift at all.

The newer contracts did real work on AWS, where 31 newly modelled operations
appeared. CloudWatch `PutLogAlarm`, five S3 object-annotation operations and six
Systems Manager cloud-connector operations were implemented against real
behaviour — the log alarm provisions a genuine scheduled query and evaluates its
state by running the configured query through the same Insights engine
`StartQuery` uses, and connector validation derives findings from real IAM state
rather than returning a canned verdict. The 19 ACM ACME operations were
classified rather than implemented and filed as BUG-2657: an ACME endpoint is
the control plane of an RFC 8555 certificate authority, so serving the CRUD
alone would advertise an endpoint resolving to nothing.

Three fidelity defects surfaced and were fixed: AWS Certificate Manager had no
tagging operations at all — masked because the conformance harness folds the
query router's unversioned bucket into every versioned service and was crediting
ACM with CloudWatch's; CloudWatch Logs Insights `count(*)` always returned zero;
and `RenameObject` dropped object annotations. Google Cloud and Azure recorded
zero spec-shape violations against the newer contracts, and Azure additionally
moved role definitions off a preview api-version that no client sends onto the
stable one they do, verified against the SDK, the CLI and observed traffic.

The vendor-tool-version skips are gone too. Eight conditionals had kept
CloudFront and CloudWatch surfaces untested on any host whose `aws` predated an
operation. The suite now declares the operations it requires, installs the AWS
CLI when the host binary lacks any, re-verifies and fails loudly if the
provisioned CLI still falls short. Only the sanctioned kernel-capability gates
remain.

What the pass could not close is recorded rather than glossed: Google serves
several Discovery revisions concurrently, so the freshness check still cannot
gate a build without flapping (BUG-2652); the coverage ratchet counts a method
as covered whenever any route pattern merely shape-matches it, so floors can
rise on documentation growth alone (BUG-2651); and the checked-in surface tables
have drifted from their generator (BUG-2659).


## 2026-07-26 — The consoles now tell the truth about what the simulators support

The three simulator consoles were badging most of their cloud's catalogue "Not
supported" while the simulators demonstrably implemented it. The AWS simulator
alone registers roughly two thousand operations across forty services, yet the
console marked EC2, DynamoDB, RDS, KMS, Route 53, API Gateway, SNS, SQS,
Systems Manager and more as unavailable. This pass derived the true map from the
simulators themselves — reading route registrations rather than file names, and
in two cases dumping the router at runtime and probing a live simulator with a
real token — then corrected every catalogue and built the missing pages.

- **AWS** — 15 of 22 "Not supported" labels were false. Fifteen further
  implemented services were absent from the catalogue entirely, and nine
  genuinely-unimplemented ones were added so the catalogue reads complete rather
  than silently omitting them. Thirty list pages and six detail pages now cover
  33 services on their real operations. A mechanical check confirmed all 89
  operations the console calls were already registered, so no simulator change
  was needed — the honest outcome rather than an invented one.
- **Google Cloud** — 19 products were falsely badged; four implemented products
  were missing from the catalogue. Twenty-one list and fifteen detail pages were
  added, with writes driven through real long-running-operation polls. One
  genuine simulator gap was filled: the regional `compute.subnetworks.list` 404'd
  while its aggregated sibling worked. `supported` is no longer asserted — it is
  derived from whether a product has a page, so nothing can claim support it
  does not have.
- **Microsoft Azure** — unsupported went from eleven to seven, with 29 new
  blades covering virtual machines, App Service, Cosmos DB, PostgreSQL, Redis,
  virtual networks, load balancers, network security groups, DNS, Key Vault,
  Service Bus, Event Hubs, Event Grid, API Management, Logic Apps and more. Two
  simulator gaps were filled (`VirtualMachines_ListAll` and
  `VirtualMachines_Update`) with SDK and CLI coverage. A follow-up corrected a
  naming error the audit exposed — the menu entry labelled "Container Apps"
  actually pointed at Container Apps *jobs* — so both now appear as the distinct
  services they are.

Screenshots of every console in both themes caught two rendering defects that
the structural, axe and contrast suites all missed. The Google Cloud header
search field carried a hard-coded light fill while its text followed the theme
token, leaving near-white text at 1.08:1 in dark mode — invisible while typing;
it now uses a themed token and measures 10.79:1. Azure's "Not supported" badge
had no style rule at all, so in the narrow service menu it wrapped onto two
lines and collided with the service name. A third defect found the same way: an
unknown `/ui/...` path rendered an empty shell in all three consoles, which now
redirect to the overview.


## 2026-07-26 — Closed the fidelity gaps the test-contract pass surfaced

The test-contract pass filed four follow-ups rather than dropping them; this
closes three and narrows the fourth to its genuine upstream cause.

- **AWS host-prefix accommodations (BUG-2648).** Three sdk-test clients
  suppressed the endpoint host prefix their operations model — Cloud Map's
  `data-`, Step Functions' `sync-`, and CloudWatch Logs' `stream-` (the third
  was not in the original report) — so the suite proved a simulator-special path
  rather than the real client path. All three now use stock clients at
  service-shaped endpoints plus a shared transport overriding only
  `DialContext`: the SDK builds and signs a byte-identical request, and only the
  dial destination differs. Two macOS `t.Skip`s in the CLI suite were removed
  the same way through an `HTTP_PROXY` coordinate, so four operations that had
  never run outside Linux CI now execute everywhere. Guard tests capture the
  signed request after the finalize step, so re-introducing the accommodation
  fails the suite.
- **Cloud Run container fidelity (BUG-2647).** `Container` now models the three
  probes and `EnvVar` models `valueSource`, with the probe/HTTP/TCP/gRPC action
  and secret-selector types taken field-for-field from the vendored Discovery
  document. Because these are the shared v2 types, Jobs, Services, Worker Pools,
  and Instances all inherit them. `EnvVar` also marshals its `values` oneof
  correctly — a sourced variable returns `valueSource` and no `value`, where the
  simulator previously always emitted an empty `value`.
- **Collapsed-port route collision (BUG-2645).** The Cloud Run Admin v1
  instances IAM aliases could not mount because Memorystore Redis owned the same
  path shape. Requests now resolve by the `Host` a real client sends, and where
  one origin serves every service the AIP-136 custom method decides — Cloud Run
  owns exactly the three IAM verbs and Memorystore owns its five actions, so the
  sets are disjoint and the resolution is total rather than a fallback.
- **Worker-pool scaling (BUG-2646) stays open, correctly.** The fields are
  modelled and covered end to end, but fetching the newest live Discovery
  document (revision 20260713) showed it still declares only
  `manualInstanceCount` — as does the published REST reference — even though
  gcloud's own generated client and the GA Terraform provider send all four
  members. This is an upstream publication lag, not a simulator defect, so the
  six resulting `unknown-field` keys are allowlisted under that bug and the
  entry stays open until Google publishes them.

Every fix was proven non-vacuous by reverting it and watching the new tests fail
— including a real `terraform plan -detailed-exitcode` returning 2 with the
missing scaling block, and drift on all three Cloud Run resources without the
probe fields. Filed BUG-2649 (roughly seven AWS CLI tests skip when the
installed `aws` predates an operation — the deceptive shape the no-skip-if-absent
rule targets) and BUG-2650 (vendored specs across all three clouds have drifted
behind upstream while the freshness check only reports, so the conformance
ratchet's authority decays silently).


## 2026-07-26 — Closed the simulator test-contract gaps, uncovering 31 fidelity bugs

`scripts/check-simulator-tests.sh` only enforces *newly added* `Register`/
`HandleFunc` lines, so handlers written before the hook could carry no SDK, CLI,
or Terraform coverage at all. Pointing the real clients at those never-exercised
surfaces is what made this pass valuable: it surfaced 31 genuine fidelity bugs,
several of which were breaking real code paths.

- **AWS Cloud Map (`cloudmap.go`) — 17 bugs.** Tagging was entirely fake
  (`TagResource`/`UntagResource` discarded input, `ListTagsForResource` always
  returned empty), which broke the ECS backend's network-state recovery — it
  finds its namespace by the `sockerless:network-id` tag. `ListNamespaces`
  ignored `Filters`, so the same backend's `TYPE=DNS_PRIVATE` query matched
  everything. `GetOperation` fabricated `SUCCESS` for any unknown operation id
  (pure synthetic behaviour) and Register/DeregisterInstance returned operation
  ids that were never stored — the two defects concealed each other. Also fixed:
  a simulator-internal Docker network name leaking onto the Namespace wire
  shape, missing pagination on five list operations, `DiscoverInstances`
  ignoring custom health status, dropped `HealthCheckCustomConfig`/SOA TTL/
  `CreatorRequestId`, missing uniqueness and validation errors, wrong
  not-found error selection, and state left behind on delete. All 30 operations
  in the servicediscovery model now have SDK coverage, 30 have CLI coverage, and
  the Terraform stack gained tags, an instance resource, a public HTTP
  namespace, and three data sources.
- **Google Cloud Run (`cloudrunworkerpools.go`, `cloudruninstances.go`) — 13
  bugs.** `instances.patch` was not implemented at all despite being in the
  Discovery document. Nine dropped-field bugs (found by running a real
  `terraform apply` → `plan -detailed-exitcode` cycle and watching it never
  converge) restored `Volume.emptyDir`/`cloudSqlInstance`,
  `SecretVolumeSource.items`, `Container.workingDir`/`dependsOn`/`baseImageUri`,
  `ResourceRequirements.cpuIdle`/`startupCpuBoost`, `VolumeMount.subPath`,
  `VpcAccess.networkInterfaces`, and several worker-pool/instance top-level
  fields. IAM verbs on a nonexistent resource returned an empty policy instead
  of `NOT_FOUND`. Most consequentially, the spec-conformance test allow-listed
  the entire `/v2/` prefix as "Artifact Registry OCI data plane", silently
  exempting every Cloud Run v2, Cloud Functions v2, and Logging v2 route from
  Discovery conformance — narrowed to the five real OCI subtree mounts, with the
  suite still green, so those routes are now genuinely checked.
- **Azure — one real bug plus the BUG-2644 coverage.** `patchWebSite` set
  `HTTPSOnly` unconditionally, so a tags-only PATCH silently cleared it; it is
  now presence-aware (absent stays unchanged, explicit `false` still applies)
  and honours the previously-ignored `enabled`/`clientCertMode`. The ACR-registry
  and Container-Apps-job PATCH handlers — correct but untested — gained SDK and
  CLI coverage, as did the Container Apps environment `/storages` sub-resource,
  ten undriven Logic Apps clients, and the capital-`F` `serverFarms` routes.

An audit heuristic that keyed on file names overstated the gap: several files it
flagged as untested were already covered, and each agent verified the real state
before working. Filed BUG-2645 (a Cloud Run v1 instances IAM alias blocked by a
collapsed-port route collision with Memorystore Redis), BUG-2646 (worker-pool
scaling fields newer than the pinned Discovery revision), BUG-2647 (Cloud Run
container probes and `EnvVar.valueSource` unmodelled), and BUG-2648 (a Cloud Map
SDK-test client pinning `HostnameImmutable` to dodge the modeled `data-` host
prefix).


## 2026-07-26 — Simulator consoles reach full functional parity (one pass)

A single comprehensive pass brought all three consoles to full functional
parity with their real cloud consoles — completing CRUD (adding the Update verb),
lifecycle actions, and the complex compute-resource creation deferred through the
incremental passes. Every flow uses real cloud APIs over the existing federated
data plane (federation/broker/signing logic untouched — only `api.ts` functions
were added); no simulator operation needed adding (the audit confirmed every
update/action/create op already existed).

- **AWS (real Cloudscape)** — Update: Lambda config (memory/timeout/env),
  CloudWatch retention, S3 versioning + tags, ECR tag-mutability/scan-on-push,
  and a reusable tags editor via `TagResource`/`UntagResource`. Actions: Lambda
  Test (Invoke). Creates: Create Lambda function (container-image or Zip-from-S3)
  and ECS Run task (existing family or an inline task definition).
- **Google Cloud (hand-built Material)** — Update: GCS bucket (storage class +
  labels), Artifact Registry (labels), Cloud Function config, Cloud Run job
  config, via real `PATCH` (with a reusable `LabelsEditor`). Actions: Cloud Run
  job Run (`jobs.run` → a real execution). Creates: Create Cloud Run job and
  Create Cloud Function. Plus a nav-drawer product search for flyout parity with
  AWS. Long-running operations driven through real `operations.get` polls.
- **Microsoft Azure (real Fluent)** — Update: a reusable tags editor via ARM
  `PATCH` on ACR/Storage/Container Apps/Functions, plus Container App job config,
  Storage account SKU/access-tier, and ACR SKU/admin-user. Actions: Function App
  start/stop/restart; Container App job Run/Stop. Creates: Create Container App
  job (ensuring the resource group + managed environment first) and Create
  Function App (ensuring a Consumption plan + runtime).

Held the bar throughout: real design tokens, light and dark at WCAG AA, axe
zero-violations on every new dialog/form/action surface in both themes, the
federated data plane untouched, real APIs only with honest error surfacing and
no fakes. Verified: all three packages typecheck/knip/build clean; new vitest
covering request shaping + form behavior for every flow (AWS 67, GCP 103, Azure
86); the package Playwright suites green with axe both themes (AWS 108, GCP 91,
Azure 77); and the Shauth relying-party matrix now runs the complete
authenticated story across all three consoles — including create → update (ECR
scan config; ACR admin) and compute-create → run (a Cloud Run job creating a
real execution). Filed BUG-2644 (an existing Azure ACR/Container-Apps PATCH
test-contract gap the work surfaced).

All three consoles are now faithful in look (real design systems on AWS/Azure,
faithful Material on GCP) and functionality (Create, Read, Update, Delete plus
lifecycle actions and compute-resource creation) against real cloud APIs.


## 2026-07-26 — Google Cloud and Azure console resource-deletion flows

Completed the consoles' create/read/delete parity: the AWS console already
deleted every resource, but the Google Cloud and Azure consoles could only
delete admin resources (projects/service accounts; app registrations/
subscriptions) — their compute and storage pages had no delete, which the real
consoles all have. Added delete (a list-page multi-select action and a
detail-page action, each opening a confirm surface that names the resource and
warns the action is irreversible) to:

- **Google Cloud** — Cloud Storage buckets (`storage.buckets.delete`, real 204),
  Artifact Registry repositories, Cloud Functions, and Cloud Run jobs (each a
  long-running operation driven through the real `operations.get` poll, reusing
  the create flow's machinery — a new `waitV2Operation` for the `/v2` functions/
  jobs collection). Fixed a real bug the delete surfaced: `authorizedJSONDelete`
  always called `response.json()`, which throws on the 204 No-Content body a
  bucket delete returns — it now returns undefined for 204 while still throwing
  the real error body on failure.
- **Microsoft Azure** — Container registries, Storage accounts, Container Apps
  jobs, and Function Apps via real ARM `DELETE` (all synchronous — the handlers
  return 200 when the resource existed or 204 when already gone, so no LRO
  polling). A shared real-Fluent `AzureConfirmDialog` backs every confirm.

Both follow the AWS delete template, preserve the federated bearer/broker/
endpoint logic (only delete functions added to api.ts), and hold the bar: light
and dark at WCAG AA, axe zero-violations on the confirm surfaces, existing tests
intact.

Verified: both packages typecheck/knip/build clean; new vitest (delete request
shaping incl. the 204-as-success handling, the LRO poll loop, and error
surfacing; plus full select→confirm→delete→invalidate round trips against a
mocked federated transport); the GCP (77) and Azure (67) package Playwright
suites green; and the Shauth relying-party matrix now runs a full create →
delete → gone round trip as the signed-in operator for a Cloud Storage bucket
and a Container registry through the real APIs, proving the authenticated delete
end to end. All three consoles now have real, end-to-end-proven Create, Read,
and Delete for these resources (Update remains the deferred piece).


## 2026-07-26 — Google Cloud and Azure console resource-creation flows

Extended the resource-creation parity started on the AWS console to the other
two, so all three consoles can create their simple resources (not just list and
inspect them):

- **Google Cloud** — the "Create bucket" (Cloud Storage) and "Create
  repository" (Artifact Registry) buttons had been disabled placeholders; they
  now open a Material `GcpDialog` wired to real `storage.buckets.insert`
  (`POST /storage/v1/b`) and `projects.locations.repositories.create`
  (`POST …/repositories`), the latter driven through a real `operations.get`
  long-running-operation poll loop the way a real client does — not an
  assume-done shortcut. GCP's wire helpers also gained a `GcpApiError` that
  parses Google's real `{"error":{code,message,status}}` body, so conflicts
  surface the real service message.
- **Microsoft Azure** — Storage accounts and Container registries gained a
  Fluent create form wired to real ARM PUTs (each idempotently ensuring the
  resource group first, as a real client does), settled synchronously the way
  the simulator's storage/ACR handlers return them (200 / provisioningState
  Succeeded). Errors surface ARM's own `error.message`.

Both follow the AWS create-flow template (a Create control opening the form,
`useMutation` over the federated path, invalidate-on-success so the resource
appears), preserve the federated bearer/broker/endpoint logic, and hold the
bar: light and dark at WCAG AA, axe zero-violations on the open create surfaces
in both themes, existing tests intact.

Verified: both packages typecheck/knip/build clean; new vitest for the request
shaping + form behavior + conflict surfacing; the GCP (60) and Azure (59)
package Playwright suites green; and the Shauth relying-party matrix now creates
a Cloud Storage bucket and a Container registry as the signed-in operator
through the real APIs and sees each appear in the list — the authenticated
create → list-refresh round trip the per-package suites cannot prove (no
identity provider) — alongside the AWS ECR-repository create proof. All three
consoles now have real, end-to-end-proven resource creation for these services.


## 2026-07-26 — AWS console resource-creation flows

Functional-parity pass: the AWS console could list and inspect S3 buckets, ECR
repositories, and CloudWatch log groups but not create them (unlike IAM and
Organizations, which already had create flows, and unlike the real AWS console).
Added a create flow to each, built with the real Cloudscape components the
console now runs on:

- **S3 — Create bucket**: `PUT /{bucket}` (CreateBucket, REST-XML) via a new
  `createS3Bucket`, with DNS-name validation and `BucketAlreadyOwnedByYou`
  surfaced.
- **ECR — Create repository**: `CreateRepository` (awsjson1.1) via
  `createECRRepository`, `RepositoryAlreadyExistsException` surfaced.
- **CloudWatch Logs — Create log group**: `CreateLogGroup` (awsjson1.1) via
  `createCWLogGroup`, `ResourceAlreadyExistsException` surfaced.

Each is a primary "Create …" button in the table header actions opening a
Cloudscape `Modal`/`FormField`/`Input`, `useMutation` over the federated SigV4
path (a new `awsRestXmlPut` helper joined the existing rest/json helpers;
`federation.ts`/`sigv4.ts` signing logic untouched), invalidating the list on
success so the new resource appears. Matches the existing IAM/Organizations
create-modal template exactly (focus trap, error `Alert`, testids).

Verified: typecheck/knip clean; vitest (6 new tests — a success and an
error-surfacing path per resource against a mocked federated transport);
Playwright 97/97 with axe zero-violations on all three create modals in both
themes; and the Shauth relying-party matrix green — it now creates an ECR
repository as the signed-in operator through the real CreateRepository API and
sees it appear in the list, proving the authenticated create → list-refresh
round trip end to end (the per-package e2e can't, having no identity provider —
its unsigned writes are correctly 403'd by the simulator's IAM enforcement).

The GCS-bucket / Artifact-Registry-repo (Google Cloud) and storage-account / ACR
(Azure) equivalents are the natural follow-ups.


## 2026-07-26 — Azure portal adopts the real Fluent component library

Mirroring the AWS console's Cloudscape migration, the Azure simulator portal
(ui/packages/simulator-azure) moved from its hand-built Fluent *approximation*
to the real `@fluentui/react-components` (Fluent UI v9 / Fluent 2 — the system
the real Azure portal is built on). React 19 compatibility was spike-verified
first (Fluent's peer range explicitly includes React 19).

The whole portal renders through genuine Fluent now: a `FluentProvider` whose
theme switches between light and dark via the shared `useTheme` hook, plus
`Toolbar`, `Popover` (the header Cloud Shell/Notifications/Help disclosures),
`Breadcrumb`, `Badge`, `Accordion` (Essentials + service-menu groups), `Table`
(all list and sub-resource tables, hand-composed from `TableRow`/
`TableSelectionCell` to keep the accessible per-row selection names), `Field`/
`Input`/`Select`/`Button` (every form), `MessageBar`, `Spinner`, and real Fluent
System Icons (`@fluentui/react-icons`) replacing the hand-drawn SVGs. The
server-side federation broker + ARM/Graph data plane (federation.ts/api.ts) were
untouched — only rendering changed.

The portal's signature header blue is the iconic Azure `#0078d4` (and its
classic hover/pressed shades) in both themes, applied by overriding Fluent's
brand-background tokens on the theme rather than accepting Fluent's stock brand
`#0f6cbd` — so the migration to real Fluent did not cost the one most
recognizable Azure colour. Light and dark both render at WCAG AA.

The migration also fixed real defects it surfaced: a `Spinner` with no
accessible name (caught by axe) and the focus-indicator expectations updated to
Fluent's real mechanism (`data-fui-focus-visible` + underline, since Fluent
zeroes the outline). A jsdom test-environment race (Fluent's tabster scheduling
a MutationObserver after teardown) was fixed with `afterEach(cleanup)` plus a
defensive `NodeFilter` polyfill in the vitest setup.

Verified end to end: typecheck/knip clean; 34 vitest; 53 Playwright (axe
zero-violations, both themes, across list/detail/not-supported/popover
surfaces); the Shauth relying-party matrix green — the Azure federated flow
(sign-in → federation broker → Entra client-secret minting → the authenticated
Container Apps job detail render) works through the real Fluent DOM, with every
RPS-critical data-testid preserved so the matrix needed no changes.

Bundle delta: dist ~384 KB → ~793 KB (112→227 KB gzip), ~2.1x — the real cost of
Fluent's Griffel CSS-in-JS runtime, tabster, and the `@fluentui/react-*`
subpackages, proportionally smaller than the AWS Cloudscape jump (~4.3x). Both
the AWS (Cloudscape) and Azure (Fluent) consoles now run on their real design
systems; Google Cloud stays hand-built (no official Google console component
library).


## 2026-07-26 — AWS console adopts the real Cloudscape component library

The AWS simulator console (ui/packages/simulator-aws) moved from its hand-built
Cloudscape *approximation* to the real `@cloudscape-design/components` — the
system the AWS Management Console itself is built on — the biggest single step
toward literal AWS console parity. React 19 compatibility was spike-verified
first (real Table/Tabs/Button render and behave under the stack).

The whole console renders through genuine Cloudscape now: `AppLayout`,
`TopNavigation`, `BreadcrumbGroup`, `Table` (with `TextFilter`/`Pagination`),
`Header`, `Modal`, `Tabs`, `Button`, `Link`, `Badge`, `StatusIndicator`,
`Alert`, `KeyValuePairs`, `ColumnLayout`, `CopyToClipboard`, `Input`,
`FormField`, `Spinner` — across the shell, all seven list pages, all five
detail pages, every modal and tab strip. Light and dark are Cloudscape's own
WCAG-AA modes via `applyMode`, wired to the existing `useTheme` hook, so
`tokens.css`/`console.css` shrank from ~600 to ~60 lines (only the always-dark
header account cluster beside `TopNavigation` and the CloudWatch Logs
transcript viewer stay hand-built). Two composites stayed hand-built for a real
reason: the searchable multi-column Services flyout (no Cloudscape equivalent)
and the side navigation (the packaged `SideNavigation` badge can't carry a
distinct "not supported" accessible name). The federated SigV4 data plane
(`federation.ts`/`api.ts`/`sigv4.ts`) was untouched — only rendering changed.

The migration also fixed real defects it surfaced: a duplicate `<main>`
landmark, an unlandmarked header account cluster, a Modal close button with no
accessible name, a dark-mode contrast failure in the Services panel, and a
duplicate not-supported aria-label.

Verified end to end: typecheck/knip clean; 49 vitest; 85 Playwright; axe-core's
full ruleset zero-violations across every page, the not-supported page, the
Services flyout, and a dialog in both themes; and the Shauth relying-party
matrix green — the AWS federated flow (sign-in → SigV4 reads → IAM key minting
→ Organizations account creation) works through the real Cloudscape DOM (the
matrix's account-row lookup was adapted to find the row by its Cloudscape link).

Trade-off, recorded honestly: the `dist` bundle grew from ~460 KB to ~2.0 MB
(JS 111→311 KB gzip, CSS 7→226 KB gzip) — the real cost of a full design system
versus a hand-rolled approximation. Scoped to AWS deliberately: Cloudscape is
open-source and the most verifiable; Microsoft Fluent (Azure) can follow, and
Google Cloud stays hand-built (no official Google console component library).


## 2026-07-25 — Simulator console parity pass 3: services flyout, ACR coordinate, authenticated detail render

Pass 3 closed the loose ends parity passes 1 and 2 left open, holding the same
bar (real design tokens, light and dark at WCAG AA, axe-clean ARIA).

- **AWS "All services" mega-menu flyout.** The real console's Services button
  now opens a full-width overlay with a live search field and the service
  catalogue in grouped columns — reusing the single `serviceCatalog.ts` (one
  supported/"Not supported" rule, applied in both the side nav and the flyout),
  with a focus trap, Escape/outside-click dismiss, and measured light/dark
  contrast. The left side nav stays the current-section affordance.
- **BUG-2643 fixed — ACR loginServer coordinate.** `simulators/azure/acr.go`
  hardcoded `loginServer` to `<name>.azurecr.io`, which no browser could reach;
  it now derives the host from the request via `azureACRLoginServer(r, name)`
  like Storage/Key Vault, so the portal's ACR detail blade resolves
  repositories/tags against the simulator. The ACA/Azure Functions overlay
  push/pull is unaffected (it uses the `SOCKERLESS_AZURE_ACR_*` coordinates, not
  `loginServer`).
- **Authenticated end-to-end detail render.** The relying-party matrix seeds a
  Container Apps managed environment and job through the real Azure Resource
  Manager API, then — after the operator signs into the portal through Shauth —
  opens that job's detail blade and asserts its live Essentials render (the
  resource group parsed from the resource id, the provisioning state the
  simulator assigned) over the federated ARM path. This closes the gap the
  earlier passes noted: detail pages were component- and structurally-tested,
  and are now proven rendering live cloud data in a real browser end to end.


## 2026-07-25 — Simulator console parity pass 2: resource detail views

Pass 2 built the resource-detail functionality pass 1 deferred (pass 1 dropped
the "View details" affordance because no detail pages existed), holding pass 1's
bar throughout — real design-system tokens, light and dark at WCAG AA with
contrast measured on painted surfaces, axe-clean ARIA, real API wiring.

- **AWS**: real API-wired detail pages for all five supported services — ECS
  task (DescribeTasks + DescribeTaskDefinition, tabbed Containers/Network/Task
  definition), Lambda function (GetFunction), ECR repository (DescribeImages),
  S3 bucket (ListObjectsV2 + GetBucketLocation), CloudWatch log group
  (DescribeLogStreams + GetLogEvents) — each with a Cloudscape details-with-tabs
  layout, "View details" restored through the table's actions render prop, and a
  new WAI-ARIA `AwsTabs` component (roving tabindex, arrow/Home/End). The
  "All services" mega-menu flyout stayed deferred (a distinct interactive
  surface), the static grouped sidebar kept.
- **Azure**: real ARM/data-plane detail blades for Container App jobs
  (executions + start/stop), Function Apps (app settings + functions), Container
  registries (repositories/tags via minted admin credentials against the ACR
  data plane), and Storage accounts (containers/blobs via a minted account SAS,
  parsing the real EnumerationResults XML) — each an Essentials grid + command
  bar + sub-resource tables. The global header gained Cloud Shell, Notifications,
  and Help as honest, accessible popover affordances (W3C ARIA APG
  menu-button/dialog pattern). Filing surfaced BUG-2643: ACR's `loginServer` is
  hardcoded to `<name>.azurecr.io` rather than derived from the request host, so
  the blade's repositories/tags panel shows a loud honest error until that
  simulator coordinate is fixed.
- **Google Cloud**: deepened the existing detail pages toward the real console
  — Cloud Run job (Details/Executions tabs with a real per-execution status),
  Artifact Registry repo (an Images tab wired to the previously-unused
  dockerImages.list), GCS bucket (an Objects tab), Cloud Function (surfaced the
  serviceConfig fields) — plus a real Cloud Logging query bar (query-language +
  minimum-severity composing the server-side filter) with entry expansion, and
  closed a gap: simulator-gcp was the only console package missing
  @axe-core/playwright, added here (which caught a pre-existing invalid-ARIA
  avatar).

Verification: all three packages typecheck / vitest / build / knip green;
Playwright e2e per package (AWS 59, GCP 50, Azure 53) covering the new
surfaces' structure, measured light/dark contrast, and axe (zero violations);
the shells were rendered in a browser in both themes and measured. Detail
pages' data-bound rendering is proven at the component level (vitest with
realistic props) and structurally in e2e; the authenticated end-to-end detail
render (which needs the federation token) remains component/structural, as with
pass 1's detail pages.


## 2026-07-25 — Simulator console parity pass 1: faithful shells, not-supported pills, light/dark, a11y

A first parity pass raising all three simulator consoles toward their real
cloud consoles' design languages, grounded in the published design systems
(not memory — the ground truth came from the real token sources), with the
rendered output verified in a browser in both light and dark.

- **AWS console → Cloudscape.** Token values read directly from the real
  `@cloudscape-design/design-tokens` package (light and dark), correcting a
  wrong link colour from a prior attempt (`#0972d3` → the real `#006ce0` /
  `#42b4ff` dark). A faithful service navigation lists nine of AWS's real
  "All services" groups; the seven supported services link to their pages and
  ~22 commonly-expected unsupported ones (EC2, RDS, EKS, VPC, …) carry an
  accessible "Not supported" pill and route to an honest not-implemented page.
- **Google Cloud console → Material 3.** A Material navigation drawer lists
  eleven real product groups with the supported/unsupported split (~25 not-
  supported chips); the theme moved onto the shared Light/Dark/Same-as-device
  hook (it previously neither persisted nor honoured the system preference);
  dialogs gained Escape/scrim/focus-return.
- **Azure portal → Fluent 2.** Token values read from the real `@fluentui/tokens`
  source, replacing Fabric-era neutrals with true Fluent 2 neutrals; the header
  blue was already Fluent's `colorBrandBackground` `#0078d4`. A service menu
  lists ten real groups (~16 not-supported badges).

Across all three: light and dark both render correctly with WCAG 2.1 AA
contrast measured against the actually-painted surfaces in a real browser
(every sampled text pair exceeded AA in both modes — AWS 17.3/11.1, GCP on
`#f0f5fe`/`#202124`, Azure 15.5/14.6); landmark roles, `aria-current`, focus
traps on dialogs, and keyboard operability were added or verified; the
not-supported affordance is conveyed non-visually (an explicit link aria-label
"<service>, not supported in this simulator" on every cloud, never colour
alone); and each package's Playwright suite gained not-supported, light/dark,
contrast, and axe-core (zero-violation) coverage. All real API wiring
(federated reads/mutations, the project picker, the Azure federation broker)
was preserved untouched.

This is an honest **pass 1**, not literal 100% parity: the shells are
hand-built approximations of the real component libraries (not the vendored
`@cloudscape-design/components` / `@fluentui/react-components`), icons are
hand-drawn in each system's style rather than the proprietary icon sets, and
per-product colour logos and some header affordances (Azure Cloud Shell/
notifications) are deliberately not replicated. Group ordering and the exact
service catalogue were built from each console's public IA, not authenticated
screenshots.


## 2026-07-25 — Azure `client_credentials` rejects unregistered clients

BUG-2639, the last actionable fidelity gap from the Console Self-Service
roadmap: the Azure simulator's v2.0 `client_credentials` grant minted a token
for any client id — an unregistered id fell through to an implicit-client
branch with no secret validation, where real Microsoft Entra rejects unknown
clients with `unauthorized_client` AADSTS700016. Enumerating every
`client_credentials`-with-secret call site showed the harnesses had almost all
already converged on one coordinate (`test-client-id`/`test-client-secret`), so
the fix was a clean single-coordinate consolidation, not a mass migration: the
simulator seeds a well-known bootstrap Entra application (that appId, a hashed
secret password credential, and its service principal — the Azure analog of the
AWS `test`/`test` bootstrap), the few stragglers (a couple of test subtests, and
a relying-party harness call that had relied on the implicit grant with no
secret at all) were pointed at it, and the implicit-client branch was deleted so
an unregistered client id now returns the real AADSTS700016. A new SDK test
asserts the rejection; the Azure Container Apps and Azure Functions backends
were confirmed unaffected (they authenticate through the managed-identity
`/msi/token` endpoint, never `client_credentials`).


## 2026-07-25 — Closed the console/simulator fidelity follow-ups the roadmap surfaced

Three fidelity gaps filed during the Console Self-Service phases, resolved
together:

- **AWS console (BUG-2637)**: `AwsTable`'s default "View details"/"Delete"
  header actions rendered enabled but did nothing on the Amazon ECS, AWS
  Lambda, Amazon ECR, Amazon S3, and CloudWatch Logs pages. Each page now
  passes the `actions` render prop so the inert defaults no longer render:
  "View details" was dropped (these resources have no detail view), and a real
  Stop/Delete was wired over the federated Signature Version 4 path (ECS
  StopTask; Lambda, ECR, S3, and CloudWatch Logs deletes) with a confirm
  dialog and the real cloud error surfaced.
- **Google Cloud simulator (BUG-2638)**: `serviceAccounts.create` silently
  overwrote an existing service account; it now returns Google Cloud IAM's real
  409 `ALREADY_EXISTS`. The Cloud Run and Cloud Run Functions backend harnesses
  that re-provision `sockerless-runner` against a persistent simulator moved to
  get-or-create, and SDK + CLI tests cover the conflict.
- **AWS simulator (BUG-2642)**, found by the Boy Scout check while fixing the
  console actions: AWS Lambda's REST API surface bypassed SigV4/IAM enforcement
  entirely — an unauthenticated call returned data, a hole in the credential
  enforcement contract. A `lambdaEnforced` wrapper (mirroring `s3Enforced`) now
  verifies the Signature Version 4 signature and evaluates the real `lambda:`
  IAM action on every control-plane route, returning Lambda's REST-JSON auth
  error shape; the `/2018-06-01/runtime/...` container-polling routes stay
  unenforced, so function execution is unaffected — proven by the invoke suite
  plus a new unsigned/wrong-secret deny test.

BUG-2639 (the Azure v2.0 implicit grant for unregistered client ids) stayed
open as a deliberate interim state: removing it is a mass migration of every
Azure test harness to provisioned app registrations, better done as its own
considered change.


## 2026-07-25 — Deployment recipe, and the Azure portal federates for real

Phase 4 of the Console Self-Service roadmap: the deployment and provisioning
recipe, and the Azure federation deployability fix (BUG-2640) it carries.

A committed `deploy/` hosts Sockerless Admin, the three simulators, and a
Shauth stack (Shauth + Ory Hydra + PostgreSQL) at persistent origins:
`compose.yaml` runs the published `ghcr.io/e6qu/*` images (a required
`SOCKERLESS_TAG`, no implicit latest), `compose.build.yaml` builds from source,
`provision.sh` registers every console as a Shauth OpenID Connect client and
provisions the cloud federation resources through the real APIs (AWS IAM OIDC
provider + roles via SigV4, GCP workforce pool providers, Azure managed
identity + federated identity credentials, capturing the generated Azure client
id into `.env.generated`), and `smoke.sh` proves every health endpoint, the
unauthenticated console redirects, each data plane's reject-unauthenticated /
answer-authenticated contract, and a `sockerless login` authorize URL. A
load-bearing discovery shaped the recipe: Admin and the consoles reject any
non-HTTPS, non-`localhost` OpenID Connect issuer, so a Caddy reverse proxy
TLS-terminates every persistent hostname on one port with its own local CA,
trusted by the backchannel-logout and federated-JWT-verifying services through
`SSL_CERT_FILE` (compose-only, no image change). The full boot → provision →
smoke passed fresh and idempotent; no CI job was added because a cold
from-source build of the five images alone runs ~10 minutes, past the
15-minute ceiling before provisioning starts, so `make deploy-smoke` is the
documented manual gate.

BUG-2640 closed: the Azure portal's Workload Identity Federation exchange moved
into the console's own **server-side broker** (`/auth/federation/token`, using
the ui-auth session's assertion) — real Microsoft Entra serves no CORS for the
`client_credentials` grant, so the browser could never read the token
response, which is why the browser-side exchange only ever worked co-served.
The Azure simulator gained faithful Azure Resource Manager and Microsoft Graph
CORS (the Entra token endpoint deliberately gets none — the reason the broker
exists), the SPA now calls the broker on its own origin and reads the cloud
cross-origin over that CORS, and the relying-party harness runs the Azure
console and cloud as **separate processes**: it provisions the console's
managed identity and federated identity credential on the cloud process, then
starts the console pointing every coordinate at it. That unblocked the Azure
browser data plane and the app-registration / client-secret **minting flow
deferred since the credential-minting phase** — the relying-party matrix now
mints an Entra client secret through the portal in a real browser, alongside
the AWS access key and Google Cloud service-account key.

## 2026-07-25 — `sockerless login` signs the terminal into every cloud

Phase 3 of the Console Self-Service roadmap: the packaged terminal analog of
`aws configure sso` / `gcloud auth login` / `az login`. `sockerless login`
(zero-dependency, stdlib-only in `cmd/sockerless`) runs the RFC 8252
native-app flow — ephemeral loopback listener, S256 PKCE, the authorize URL
printed (and opened unless `--no-browser`), Shauth sign-in and the one-time
consent screen in the operator's browser — then wires **vendor-native
credentials** that the vendor tools refresh themselves, never one-shot copied
secrets:

- **AWS**: an INI-preserving `~/.aws/config` profile with `role_arn`,
  `web_identity_token_file`, `region`, and `endpoint_url` — the AWS CLI runs
  `AssumeRoleWithWebIdentity` itself (`aws --profile sockerless-<ctx> sts
  get-caller-identity` returns the assumed federation role).
- **Google Cloud**: a real workforce `external_account` Application Default
  Credentials file plus a dedicated gcloud configuration with the proven
  `api_endpoint_overrides`, activated via `gcloud auth login --cred-file`.
  Proving this surfaced BUG-2641: gcloud resolves the signed-in account via
  STS token introspection (`POST /v1/introspect`, RFC 7662), which the
  simulator lacked — implemented against gcloud's captured live wire (HTTP
  Basic with Google's published gcloud OAuth client, `principal://…/subject/…`
  username, `active:false` for unknown tokens as real Google answers) with
  SDK-shaped and real-gcloud CLI coverage.
- **Microsoft Azure**: `az cloud register` + `az login --service-principal
  --federated-token` — az stores the assertion and re-exchanges it on demand.
  az/MSAL reject any http authority, so the relying-party harness runs a
  second TLS-serving Azure simulator instance for the CLI's coordinates.

Shauth findings baked into the harness: the CLI registers as a public Hydra
client (`token_endpoint_auth_method: none`) with the RFC 8252 loopback
any-port redirect; non-managed clients traverse Shauth's explicit consent
screen once. `sockerless logout` removes the token, the ADC file, and the
CLI's own profile section, and runs `az logout`. The relying-party matrix
drives the whole story: spawn the CLI, sign in and authorize in a real
browser, then prove `aws`, `az`, and `gcloud` each work vendor-natively
against the simulators, then log out.

## 2026-07-25 — Consoles manage accounts, projects, and subscriptions

Phase 2 of the Console Self-Service roadmap: a Shauth-authenticated operator
manages the account containers themselves, through each cloud's real APIs over
the federated session.

- **Google Cloud** gained a real Cloud Resource Manager slice — and building
  it surfaced that the existing partial v3 surface was faked: the sim
  synthesized an ACTIVE project for any never-seen ID, returned a synthetic
  done-operation for any operation name, never enforced duplicate-ID 409s, and
  used the wrong v3 `name` form. All replaced with the real contract, verified
  against what the real clients actually speak (gcloud's CRM v1 with its
  `lifecycleState:ACTIVE` filter; Terraform's v1 lifecycle plus an
  unconditional Cloud Billing read honoring `cloud_billing_custom_endpoint`;
  the v3 GAPIC client whose proto resolution rejects invented operation
  metadata types — each operation now carries its verb's real metadata
  message). v1 create/list/update/delete/undelete/operations plus Cloud
  Billing `getBillingInfo` were added, projects resolve by ID or number,
  unknown projects answer 403 `PERMISSION_DENIED` without disclosing
  existence, and delete is the real 30-day soft-delete. The console gained the
  real header project-picker chip (search, New Project driving the create
  LRO, Manage resources page with Shut down) and every console page now reads
  the selected project — the hardcoded project constant is gone. SDK, gcloud
  CLI, and `google_project` Terraform coverage landed in the same change,
  including a real-provider apply/plan-idempotency/destroy proof.
- **Microsoft Azure** gained the Microsoft.Subscription alias API at 2021-10-01
  Swagger fidelity (vendored): alias PUT (billing-scope creation and
  subscription adoption), the documented provisioning-state polling model
  (verified against both azcore's body poller and go-autorest's), rename/
  cancel/enable, and created subscriptions backing `GET /subscriptions`. The
  portal gained a Subscriptions blade (list, Add with live provisioning,
  detail with Cancel/Reactivate — no invented delete; Azure has none).
  armsubscription SDK, az CLI (`az rest` on the documented wire — the alias
  commands live in a preview extension), and `azurerm_subscription` Terraform
  coverage (both modes) landed together; the subscription resources run as
  their own `tf (azure subscription)` CI shard because the provider's fixed
  60-second settle delays don't fit the shared azure stack's budget.
- **AWS** gained the Organizations console page — accounts table, the real
  console's asynchronous "Add an AWS account" flow (`CreateAccount` polled via
  `DescribeCreateAccountStatus`), the organization-not-in-use state with
  "Create an organization", remove/close actions, and account detail — over
  the existing Organizations slice. `awsJson` now surfaces the real awsjson1.1
  error code so pages branch on the service error, and the relying-party
  federation role gained `organizations:*`.

The Shauth relying-party matrix drives both new browser flows (create an
organization account to SUCCEEDED; create a project through the picker and
switch to it) and passed with its exit code observed directly.

## 2026-07-24 — Consoles mint real CLI credentials for Shauth-authenticated users

Phase 1 of the Console Self-Service roadmap: each simulator console gained its
cloud's real credential pages, driven by the operator's federated credentials
calling the cloud's real APIs — never a console-only endpoint.

- **AWS**: an IAM Users page and a per-user Security-credentials page (AWS
  Identity and Access Management `CreateUser`/`CreateAccessKey`/`ListAccessKeys`
  /`DeleteAccessKey`/`UpdateAccessKey` over the AWS Query protocol, SigV4-signed
  browser-side with the federated temporary credentials). "Create access key"
  reproduced the real console's one-time disclosure — the secret is viewable
  exactly once, masked behind Show/Hide, with the exact `aws configure` values
  and an endpoint-scoped `aws sts get-caller-identity` verification command. A
  CLI test proved the loop: a key minted via `aws iam create-access-key`
  authenticated `aws sts get-caller-identity` (returning the minted user's ARN)
  and a wrong secret failed with `SignatureDoesNotMatch`. The header/Overview
  Region badges now render the same Region every SigV4 signature scopes to.
- **Google Cloud**: Service Accounts list/create/delete and a Keys tab
  (`serviceAccounts`/`keys` IAM APIs over the federated bearer) with the real
  console's one-time JSON key download (`privateKeyData` decoded to
  `<project>-<keyid>.json`, unrecoverable afterwards) and a gcloud usage panel
  proven verbatim by a CLI test (`gcloud auth activate-service-account` with a
  minted key authenticates; a tampered key fails with `invalid_grant`). Fixing
  the end-to-end loop surfaced that the simulator's token endpoint **never
  verified assertion signatures** and discarded every minted key's public half:
  keys.create now registers the public key, deletion revokes it, and the OAuth
  2.0 token endpoint verifies the RS256 signature, expiry, and account state
  exactly as Google does — the backend harnesses' self-keypair helpers were
  deleted and moved to the real mint flow.
- **Microsoft Azure**: App registrations and Certificates & secrets blades
  (Microsoft Graph `applications`/`servicePrincipals` routes with a
  Graph-scoped federated token). The real portal mints secrets on the
  application object, so the simulator gained faithful Graph
  `applications/{id}/addPassword`/`removePassword` (secretText returned exactly
  once, SHA-256 verifier stored). Tracing validation found the v2.0 token
  endpoint **checked no client secret at all**; `client_credentials` for
  directory-registered applications now validates the secret and returns real
  AADSTS error shapes, proven by SDK and az-CLI tests (mint → ARM read; wrong
  and revoked secrets rejected). The sockerless-invented `/sim/v1/entra/users`
  routes and the `entraActiveOID` global were deleted; consumers migrated to
  Graph `POST /v1.0/users` provisioning and `login_hint` binding.

The Shauth relying-party browser matrix gained minting flows for the AWS and
Google Cloud consoles: the signed-in operator drives the real UI to mint a
credential over the federated session, and the one-time disclosure semantics
are asserted (secret gone after dismissal). Getting those flows green surfaced
two environment defects the suite had been silently missing: the harness's
console federation role lacked `iam:*` (the simulator's IAM enforcement
correctly denied the minting pages, exactly as real AWS denies an operator
role never authorized for the IAM console), and full-page navigations in the
new flows aborted the prior page's in-flight reads (the flows now navigate
through the console's own navigation, as an operator does). The Azure portal
got no browser-driven minting flow: its browser-side Workload Identity
Federation exchange is same-origin-only (real Microsoft Entra serves no CORS
for `client_credentials`), and the relying-party environment cannot provision
the portal's managed identity before console start — filed as BUG-2640 with
the deployment-phase fix shape (server-side federation broker + faithful CORS
+ separate console/cloud processes); Entra minting itself is proven end to end
by the Azure SDK and az CLI suites. BUG-2637 (inert default table actions),
BUG-2638 (`serviceAccounts.create` overwrite instead of 409), and BUG-2639
(implicit grant for unregistered client ids) were filed with fix shapes.

The `sim (aws sdk)` job — chronically within two minutes of the enforced
15-minute ceiling — hit it on runner variance and was split into four shards
(`compute` = `^Test[E]`, `data` = `^Test[D]`, `services-a-m` = `^Test[A-CF-M]`,
`services-n-z` = `^Test[N-Z]`),
mirroring the AWS CLI shards: shard regexes use the character-class form so
each suite's coverage gate reads only its own shard set,
`scripts/check-sdk-shard-coverage.sh` (pre-commit) asserts all 1152 SDK tests
match exactly one shard, the DynamoDB Local oracle pull rides the data shard
and the module unit tests the services-n-z shard, and
`.github/required-status-checks.txt` carries the four new contexts (branch
protection must swap `sim (aws sdk)` for them when this merges).

## 2026-07-24 — Closed the skip-if-absent gate hole and swept the last tool-absent skips

`scripts/check-no-tool-absent-skips.sh` only rejected `t.Skip`/`t.Skipf` lines, so
a TestMain-level `exec.LookPath(tool)` → `fmt.Println("… not found, skipping")` →
`os.Exit(0)` evaded it silently — which is how the Google Cloud and Azure CLI
suites came to skip themselves whenever `gcloud`/`az` were absent. The gate now
also rejects (2) a `fmt.Print*`/`log.Print*` line carrying a tool-absent phrase
and (3) a bare `os.Exit(0)` within a few lines of a `LookPath(` in the same hunk,
and it exempts skips that self-identify as platform/kernel-capability gates
(`runtime.GOOS`, `CAP_NET_ADMIN`, "requires a Linux host") so a legitimate GOOS
gate whose message happens to say "not available" is not a false positive.

Every remaining tool-absent skip was resolved to install-or-fail-loud:
- The Google Cloud CLI suite installs a pinned Google Cloud CLI release into a
  temp dir when `gcloud` is absent (mirroring the AWS suite's
  `installLatestAWSCLI`); the Azure CLI suite fails loud with an actionable
  message — the `az` Python application has no clean cross-platform TestMain
  install — each replacing its `os.Exit(0)` skip.
- The six `t.Skip("docker CLI not available")` guards in the AWS ECS VPC-networking
  CLI tests and the one in the Azure Cosmos differential test were vestigial:
  their TestMains already `docker build` images and `log.Fatalf` without Docker,
  so Docker is guaranteed present before any test runs. They were removed; the
  Linux + CAP_NET_ADMIN netns capability gates stayed.
- The `session-manager-plugin` (AWS ECS execute-command), `git` (AWS Amplify),
  `gcloud` (Google Cloud Firestore differential), and `nsenter` (realexec
  external-namespace round-trip) skips became `t.Fatal`. Each is a tool the
  relevant CI job already provides, so CI stayed green while a local run without
  it now fails loud with an actionable message instead of skipping unseen.

## 2026-07-24 — Gated required-check drift so a job rename can't stall the merge queue

Splitting the AWS CLI groups into shards once renamed their jobs while `main`'s
required-status-check list still demanded the old contexts, which could never
report again — so every pull request stalled as pending (BUG-2633). The list was
corrected then, but nothing prevented a recurrence.

`.github/required-status-checks.txt` is now a version-controlled manifest of the
required contexts, and `scripts/check-required-status-checks.sh` — wired into
pre-commit and the `build-gates` CI job — enumerates every check name any
workflow in `.github/workflows/*.yml` can emit, rendering each job's `name:`
template over its matrix (handling the inline-list, block-list, and `include:`
matrix forms), and fails when a required context is no longer emittable. So a
matrix job rename now fails the pull request that causes it, with the manifest as
the reviewable bridge to the branch-protection update. A maintainer-run
`--verify-branch-protection` mode reconciles the manifest against live branch
protection (failing loudly rather than skipping when admin credentials are
absent). The manifest matched all 39 current required contexts, and a negative
test confirmed the gate flags a renamed shard.

## 2026-07-23 — Made the simulators verify credentials, and fixed the ECS harness

The simulators accepted unverified caller-controlled credentials (BUG-2625, P0):
AWS derived identity from the cleartext `Credential=` access-key id and never
checked the SigV4 `Signature`; Google Cloud and Azure trusted arbitrary bearer
content (GCP had no data-plane auth at all; Azure verified a bearer only on its
UserInfo endpoint). All three now verify the way the real clouds do.

- **AWS** recomputes the SigV4 signature (canonical request, key derivation,
  constant-time compare) at the awsjson/query control plane and S3, looking up
  the secret for the presented long-term (`AKIA`) or temporary (`ASIA`) key —
  temporary-credential secrets and session tokens are now persisted, and a
  bootstrap account credential (`test`/`test`, the coordinate every client
  already signs with) is seeded so an account can act before it mints its own
  keys. Failures return `SignatureDoesNotMatch` / `InvalidClientTokenId` /
  `MissingAuthenticationToken`. `AssumeRoleWithWebIdentity`/SAML, presigned
  URLs, and S3 public reads stay exempt. Enforcement also exposed and fixed a
  synthetic-behavior bug: the Amplify handler had minted a fake presigned S3 URL
  with no signature (BUG-2636).
- **Google Cloud** consolidated its access-token minters onto one process-stable
  RS256 key, published a JWKS, and added a data-plane middleware verifying the
  bearer's signature, issuer, audience, and expiry (`UNAUTHENTICATED`
  otherwise). **Azure** reused its RS256 verifier, added the missing audience
  check, and wired it as an ARM data-plane middleware (`invalid_token`
  otherwise). Both exempt their token minters, discovery/JWKS, metadata/IMDS,
  health, and OCI registries.

Because the simulators now enforce, every consumer that had relied on them not
checking was made faithful: the SDK/CLI/Terraform suites fetch a real token or
sign with the seed; the sockerless backends dropped their `WithoutAuthentication`
(GCP) and `fakeCredential` (Azure) fakes for a real GCE metadata token source
and `DefaultAzureCredential` — differing from the real cloud only in
coordinates; the relying-party suite signs its AWS IAM provisioning and bearer-
authenticates its GCP and Azure provisioning; and the console browser e2e, which
reaches the enforcing simulator without an identity provider, moved its
reads-real-data assertions to the authenticated relying-party path.

Separately, the AWS ECS Terraform harness left subprocesses running past its
deadline (BUG-2569, P1). The `internal/tfsim` harness gained the process-group
and deadline-watchdog reaper the main harness already had, and both service
fixtures and production-shaped provider graphs used explicit long-running
workload commands plus deterministic cleanup. `TestStackProductionShape`
converges and terminates with zero leaked containers or processes while the
service scheduler runs its declared tasks.

## 2026-07-23 — Completed the Microsoft Azure portal on both fidelity axes

The Azure portal got the same treatment the Google Cloud and AWS consoles did,
completing the set: every simulator console now reads only real cloud APIs, and
no `/sim/v1/*` dashboard endpoint remains on any of them.

Data: the portal reads the real Azure Resource Manager APIs — Azure Container
Apps jobs, Azure Functions sites, Azure Container Registry, and Azure Storage
accounts — enumerating subscriptions and listing each provider across them, plus
Azure Monitor's Log Analytics query API (a distinct host and token audience from
Azure Resource Manager, reached by listing Log Analytics workspaces and running a
Kusto query against each). The invented `/sim/v1/*` dashboard endpoint is
deleted. The operator's Shauth assertion is exchanged through **Microsoft Entra
Workload Identity Federation** — the `client_credentials` grant with a JWT-bearer
`client_assertion` — against a registered federated identity credential; the
simulator now verifies the assertion against the identity's credential issuer,
subject, audience, and RS256 signature (via `go-oidc` + `go-jose`) and issues an
Azure token, where it previously issued one to any client_credentials request.
This is the same client code and identifiers a real client uses against Azure,
differing only in the endpoint, tenant, and identity coordinates, with no
sim-aware branch. The relying-party suite proves the whole path with a live
Shauth operator token: an administrator registers a managed identity and a
federated identity credential trusting the operator's own issuer, subject, and
audience, and Microsoft Entra exchanges the operator's assertion for an Azure
Resource Manager token.

Visual: the portal was already built to the Azure portal's layout — the blue
header, the "Microsoft Azure" wordmark, the command bar, the Essentials strip,
and the grouped service menu. It got a **Fluent-style inline-SVG icon set**
(approximating Fluent UI System Icons, MIT, drawn in-repo and self-contained) on
the command bar, the status pills, the service-menu and Essentials chevrons, the
header search, and the theme control, replacing the placeholder Unicode glyphs.
The browser suite pins the header blue and the command, status, and search icons
structurally so the Azure look cannot regress unseen.

## 2026-07-23 — Completed the Amazon Web Services console on both fidelity axes

The AWS console got the same treatment the Google Cloud console did, in one
branch covering both axes: real cloud data and the real visual language.

Data: the console reads the real AWS APIs — Amazon ECS, AWS Lambda, Amazon
ECR, Amazon S3, and Amazon CloudWatch Logs — over the console's own
server-side Shauth federation, replacing an invented `/sim/v1/*` dashboard
endpoint that was deleted. The operator's Shauth id_token is exchanged through
`AssumeRoleWithWebIdentity` against a registered IAM OpenID Connect provider;
the simulator now verifies the web identity token against that provider's
issuer, audience, and RS256 signature and reports the token's real subject
(it previously returned a hardcoded one). Each read is signed in the browser
with Signature Version 4 over the returned session credentials using Web
Crypto — the same client code and identifiers a real client uses against AWS,
differing only in the endpoint and federation coordinates, with no
sim-aware branch. ECS tasks are enumerated the way the real console shows
them: list clusters, then per-cluster list and describe. The relying-party
suite exercises the whole path with a live Shauth identity and a role carrying
a read policy, so an unpermitted role reads as the real 403 rather than silent
empties.

Visual: the console was rebuilt to the Cloudscape Design System, graded
side-by-side against the live reference with tokens read from its computed
styles rather than from memory. Open Sans — Cloudscape's own console font,
since Amazon Ember is proprietary and is approximated, said plainly — is
vendored as a woff2 subset; Cloudscape's action blue and rounded containers
are applied from the design system's own values; the dark navigation header
carries a global search field and a tool cluster (notifications, settings,
support, theme); and a Cloudscape-style inline-SVG icon set (SIL OFL / drawn
in-repo, self-contained) sits on the status pills and the table's
search-prefixed filter and refresh control. The browser suite pins the font,
the action blue, the container radius, and the header and table controls
structurally so the AWS look cannot regress unseen.

## 2026-07-23 — Rebuilt the Google Cloud console to the console's real visual language

The console recognisably evoked Google Cloud but sat far below what was
achievable — almost no icons, a fallback typeface, generic glyphs, and an
"identity unavailable" error rendered into its own chrome. A side-by-side
against the live console made the gap plain, and it turned out I had been
grading against memory with structural-only tests and an overclaimed "presents
as". This closed most of the gap, working from the console's own computed styles.

Design tokens are the values the live console paints — its blue-tinted page
background, the left-anchored active pill and its colour, the primary blue.
Roboto is vendored as a latin woff2 subset for body text (the display face,
Google Sans, is not redistributable and is approximated, said plainly); icons
are Material Symbols Outlined vendored as inline SVG paths, so the console is
self-contained — a real icon on every navigation item, the header tool cluster,
the filter, sort and column controls. Both are Apache-2.0. The account is an
avatar opening a menu with the identity and sign-out, neutral rather than an
error when unauthenticated; the empty state is completed. The browser suite now
pins the visual work structurally so it cannot regress to a sketch unseen. The
information architecture stays a deliberate divergence — one rail for the
resources the simulator implements rather than the real per-product navigation.

## 2026-07-23 — Read the last Google Cloud resources from their real APIs

Cloud Run jobs already read the real Cloud Run API; the console's other four
resources still read sockerless-invented `/sim/v1/*` endpoints with a trimmed
shape. They now read their real cloud APIs through the same federated,
coordinate-only path — Cloud Run functions from Cloud Functions v2, Artifact
Registry from its repositories, Cloud Storage from the JSON API, and Cloud
Logging from `entries:list` — each with a detail page on the real resource and
each rendering the true shape.

The overview counts each resource from the same real list its page reads,
rather than a summary endpoint, and reports whether those APIs answered rather
than a synthetic health signal. With the last consumer gone, the Google Cloud
dashboard — every `/sim/v1/*` route including the summary — is deleted; the
console reaches the cloud only through real APIs at configured coordinates. The
browser suite creates a bucket, a repository, and a log entry through the real
APIs and asserts the console lists each and opens its detail; the relying-party
suite is unchanged and still green.

## 2026-07-23 — Made the Google Cloud console reach the cloud only through real APIs and coordinates

The console had federated the operator through `/auth/cloud-token`, an endpoint
the simulator served and the real cloud does not — coupling the data plane to
the simulator, so the same console pointed at real Google Cloud would have
needed a simulator-versus-cloud branch. The console now reaches the cloud
exactly as it would reach the real thing, differing only in coordinates.

The console's Shauth authentication is the console's own layer, not the
simulator's. It stays server-side — session, front- and back-channel logout,
the marker contract — and exposes the operator's assertion to the browser at
`/auth/federation-subject`. The browser federates that assertion at the cloud's
real Security Token Service (`POST /v1/token`) and calls the real cloud APIs
with the result. The Security Token Service endpoint, the cloud API base, and
the workforce pool provider the console federates through are coordinates the
console reads from its configuration; empty means its own origin, where the
simulator serves them, and a real deployment points them at Google Cloud.

The simulator-served credential broker and its sim-side workforce-provider
auto-provisioning were deleted. The provider is provisioned the way an
administrator provisions it — through the real Identity and Access Management
API — by the relying-party harness standing in for the administrator, and its
resource name reaches the console as a coordinate.

The relying-party suite reads the assertion from the console's auth layer,
exchanges it at the real Security Token Service with a live Shauth identity, and
drives the signed-in console through a real Cloud Run API read over that
federation. Login, logout, and the marker contract are unchanged, so the same
suite proves no regression. The rule is written in AGENTS.md: a simulator
console UI differs from a real-cloud console only in coordinates.

## 2026-07-23 — Read the real Cloud Run API from the Google Cloud console, over Shauth federation

The simulator consoles looked like their clouds but read data through
sockerless-invented `/sim/v1/*` endpoints that returned a hand-trimmed shape.
The Google Cloud console's Cloud Run jobs view now reads the real Cloud Run
Admin API — the `/v2/.../jobs` list and the job resource behind a new detail
page — and renders the true resource: status from the job's terminal condition,
unique ID, launch stage, timestamps, labels, and executions. The invented
endpoint was deleted.

A real console reaches those APIs with a credential federated from the signed-in
session, so the simulator gained the pieces it was missing. The Security Token
Service token exchange (`/v1/token`) performs Workforce Identity Federation the
way `sts.googleapis.com` does: it resolves the workforce pool provider the
audience names, verifies the subject token against that provider's OpenID
Connect issuer with real discovery, key set, signature, issuer, audience, and
expiry checks, and issues a short-lived federated access token. A console
credential broker on the operator-session boundary reads the operator's Shauth
assertion — already captured in the ui-auth session — and exchanges it for that
token, which the browser presents as a bearer credential. The raw assertion
never leaves the server, exactly as a real console keeps it.

Whether a credential is attached is a real deployment condition rather than a
fallback: a simulator wired to a single sign-on provider federates the operator
and every call carries a token, surfacing a broker failure; a simulator with no
identity provider runs unauthenticated, the mode the account control already
reports.

The token exchange is driven end to end by the official external-account
credential in the SDK tests, with the refusals that matter. The browser suite
seeds a job through the real Cloud Run API and asserts the console lists it and
opens its detail, proving live resources render. The relying-party suite drives
the whole federation with a live Shauth identity, brokering the operator's
assertion into a cloud token and checking a bearer token returns. The credential
issuance advanced BUG-2625; the remaining Google Cloud resources and the AWS and
Azure slices follow the same pattern (BUG-2635).

## 2026-07-23 — Gave the Google Cloud simulator the Google Cloud console's interface

The last of the three simulator interfaces now presents as its own cloud's
console, so an operator who knows any of AWS, Azure, or Google Cloud recognises
the simulator for that cloud.

The reference was the real Cloud Run Jobs page, captured from the console
itself: a light global header with a project chip and a wide central search, a
product navigation whose active item is a filled pill, inline text actions
beside the page title rather than a button group, a refresh pinned at the
right, a description sentence beneath the title, and a filter chip above a
table whose headers carry inline help. Empty states pair a dashed-cloud
illustration with a headline, an explanation, and the side effect of the
primary action, matching what the console shows when a resource has none.

Tables keep their column headers while loading, empty, or failed, so what the
resource is described by stays readable when there are no rows to infer it
from. Cloud Logging omits severity when it is the default, and the console
reads that as DEFAULT rather than a blank cell.

Both themes are carried, with the control in the top right. Contrast was
measured against the surfaces the browser actually paints rather than assumed
from the palette, and a test holds every enabled text role at or above the
4.5:1 WCAG AA requires — disabled controls excluded, since the requirement
exempts them and the console greys them deliberately. The link blue was moved
one step darker so a text action clears AA on white; it had measured 4.27:1.
The test names the role and the ratio when a colour regresses.

The header sizes to its contents and establishes a stacking context, and a
test asserts that every control it holds lies inside it — the property whose
absence let a click reach the breadcrumbs on the AWS console. Status is matched
on whole words, since a substring test reports success for failure states. The
table is built in the console's idiom rather than from a generic table library,
which the package no longer depends on.

## 2026-07-22 — Gave the Azure simulator the Azure portal's interface

The second of the three simulator interfaces now presents as its own cloud's
console. Microsoft publishes an annotated diagram of the portal shell, and the
simulator follows it: the blue global header with a wide central search, a
breadcrumb, a resource title carrying the resource type and the directory it
belongs to, a horizontal command bar of icon actions with unavailable commands
greyed rather than hidden, and a service menu with its own search and
collapsible groups. A search narrows the menu to what matched, opening a group
the operator had collapsed rather than hiding the match inside it.

Every resource pane leads with Essentials, the portal's two-column key/value
grid, before its table. Resource tables carry per-column sorting, selection,
filtering, and pagination, and keep their column headers while loading, empty,
or failed, so what the resource is described by stays readable when there are
no rows to infer it from.

Essentials states only what the query returned. An earlier draft asserted a
constant "Available" beside every resource, which would have reported health
the simulator had never been asked for.

Both themes are carried, with the control in the top right. Contrast was
measured against the surfaces the browser actually paints rather than assumed
from the palette, and a test holds every text role at or above the 4.5:1 that
WCAG AA requires — the tightest is 4.53:1, white on the portal's own header
blue, which leaves little room to drift. That test fails, naming the role and
the ratio, when a colour regresses.

The header sizes to its contents and establishes a stacking context, and a
test asserts that every control it holds lies inside it. On the AWS console a
fixed-height header left the sign-out control drawn outside the header box
where the bar below covered it, and clicks aimed at it reached the breadcrumbs
instead. The same shape of bug is now checked for here rather than waited for.

## 2026-07-22 — Reclaimed the microVM workspaces that filled the runner volume

The `sim (aws sdk)` job began with 89 GB free after its own cleanup and still
exhausted the runner volume, killing the runner process as it wrote its own
diagnostic log. Nothing identified the writer, because uploading the job log
needs the disk that was full.

The step now writes test output to a file under a watched budget and reports
the largest consumers on the volume when a threshold is crossed — while there
is still disk to report them on. That named the consumer immediately:
`/tmp/sockerless-firecracker` held 24.7 GB of the 26 GB a passing run consumed.

Each Firecracker machine staged a full copy of the root filesystem tree in
order to build an ext4 image from it, then kept both for as long as the machine
ran. The staging tree is now removed once the image exists, since nothing reads
it afterwards.

A machine that is killed rather than stopped never ran its own cleanup, so its
workspace — a root filesystem image each — stayed for the life of the host. A
machine now records which process its workspace belongs to, and a later machine
reclaims workspaces whose owner is gone. Workspaces too young to have recorded
an owner are left alone, so a machine on its way to starting is not swept out
from under itself.

## 2026-07-22 — Gave the AWS simulator the AWS console's interface

The three simulator interfaces were one generic application wearing three
accent colours: identical layout, navigation, typography, and components, with
only the tint and the navigation labels differing. An operator who knew any of
the real consoles recognised nothing.

The AWS simulator now presents as the AWS Management Console. It carries the
dark global header, a breadcrumb trail, a service navigation that groups
services the way the console groups them, `Resources (count)` page headers
beside their actions, and resource tables with per-column sorting, selection,
filtering, pagination, and the console's own empty states naming the resource
and the Region. The service overview states Region and service health before
counts, and each count links through to the resource that owns it.

Values the previous interface dropped are shown: log retention, stored bytes,
creation timestamps, and function state.

Both themes are carried, with the control in the top right of the header where
the console keeps it. Every text and surface pair was measured against the
rendered result rather than assumed: the lowest ratio is 4.97:1 in light and
5.25:1 in dark, against the 4.5:1 that WCAG AA requires for body text.

Status is matched on whole words. A substring test reported success for
failure states, because "unavailable" contains "available" and "inactive"
contains "active", and a green tick on a failed resource stops an operator
looking further. Callers that know the meaning pass it rather than having it
inferred from wording.

## 2026-07-21 — Qualified the real product user interface, not a protocol page

Sockerless Admin and the AWS, Google Cloud, and Microsoft Azure simulator
dashboards exposed the authenticated operator through
`data-shauth-user="<exact username>"` on the visible account control and
`data-shauth-sign-out` on the real sign-out control. The browser qualification
matrix asserted the visible username against the identity endpoint and signed
out by clicking the control a person clicks, so a deployment whose product
shell renders no user or no sign-out control failed qualification even when its
protocol endpoints answered correctly. The markers carried no authorization
meaning and replaced no accessible name or semantic element.

A stale required-status-check list on `main` still demanded the pre-shard
`sim (aws cli edge)` and `sim (aws cli ec2)` contexts, which the four-shard
matrix could no longer emit; the list was corrected to the shard contexts and
the drift was recorded as BUG-2633.

## 2026-07-21 — Completed the exact Shauth and bounded-release contracts

Sockerless Admin's logout-completion bridge remained public after local session
revocation and redirected only to Shauth's issuer-correlated completion
endpoint. Admin and the AWS, Google Cloud, and Microsoft Azure simulator
dashboards passed the exact Shauth `0fda680cba964e5768ed75a9c3e5b7230c418ca6`
contract against real PostgreSQL, Ory Hydra, freshly compiled relying parties,
and Chromium. Eight serialized application-and-direction flows proved direct
and catalog entry, relying-party and provider logout, exact completion
bridging, application-local signed-out return, reload, reauthentication,
global revocation, immutable release identity, anonymous fail-closed behavior,
active event-stream readiness, and validator-credential isolation.

The production build created every frontend bundle before compiling all 11
UI-bearing Go binaries, and a repository gate rejected ordering regressions
that could silently produce headless release artifacts. Every ordinary GitHub
Actions job declared an enforced timeout of at most 15 minutes. Historical
runtime evidence split the over-budget AWS edge and Amazon EC2 command-line
interface groups into four non-overlapping shards while preserving exact
single coverage of all 630 AWS CLI tests.

The nightly fuzz harness ran targets in bounded parallel batches with one Go
fuzz worker per target, retained truthful logs and crasher handling, and failed
on missing modules instead of skipping them. A complete one-second pass
exercised every discovered target in the AWS, Google Cloud, and Microsoft Azure
simulators and shared modules, core, Docker backend, and agent. The complete
test, lint, clean production-build, pre-commit, real authentication, and fuzz
gates passed together.

The required pre-push freshness gate also advanced every tracked Google Cloud
Storage consumer to v1.64.0. The complete Google Cloud Run, Google Cloud Run
Functions, shared Google Cloud backend, and standalone Google Cloud simulator
SDK suites passed with the reconciled module graphs.

## 2026-07-20 — Made simulator registry pushes portable and faithful

The Google Cloud Build and Azure Container Registry Tasks official SDK
harnesses shared one container-engine registry-policy utility. Docker Engine
continued to trust HTTP loopback registries natively, while Podman received an
exact scoped registry policy and reloaded that policy before the real build and
ordinary Docker-compatible push. Cleanup removed only the test-owned policy and
reloaded Podman again. The complete Google Cloud and Microsoft Azure official
SDK suites passed on macOS Podman, including real registry manifests and image
cleanup.

## 2026-07-20 — Scoped dependency freshness to repository source

The mandatory dependency freshness gate enumerated Git-tracked Go modules,
Terraform provider declarations, and GitHub Actions instead of walking
arbitrary nested directories in the worktree. User-owned untracked worktrees
therefore could not contaminate a repository release gate. The same pass moved
all three Google Cloud Secret Manager consumers and every workflow checkout
action to their current published releases, with the affected module graphs and
checks reconciled.

## 2026-07-20 — Made signed-out Shauth re-entry explicit

Sockerless Admin and the AWS, Google Cloud, and Microsoft Azure simulator
terminal pages exposed accessible, keyboard-visible `Sign in with Shauth`
controls instead of generic return actions. Admin linked to `/auth/shauth`; each
simulator linked to `/auth/oidc/login`. The standalone responses retained
no-cache headers, responsive layouts, semantic status text, and automatic
light/dark rendering.

The real PostgreSQL, patched Ory Hydra, Shauth, compiled four-relying-party,
and Chromium matrix logged out from every application, proved cross-application
session invalidation, exact app-local landing, and reload persistence, then
validated and clicked each exact Shauth control. Focused Go and Playwright
coverage locked the same labels and coordinates into each owning component.

## 2026-07-20 — Enforced Sockerless Admin administrator authorization

Sockerless Admin required the Shauth `admin` role at the one middleware boundary
shared by its operator user interface and APIs. An authenticated developer
received a no-cache accessible `403` page with a logout control, while API
requests received a JSON `403` before an operator handler ran. Administrator
sessions retained the complete operator surface.

Focused coverage drove the real topology manager and filesystem, proving a
developer could not persist a project while an administrator could. The full
PostgreSQL, patched Ory Hydra, Shauth, compiled relying-party, and Chromium
matrix also created a developer through Shauth's own administration interface,
authenticated both roles through the real OpenID Connect flow, proved the
developer denial, and persisted and removed an administrator-owned topology
project. The harness ran Admin from an isolated temporary working directory so
its real persistence proof never changed the repository topology.

## 2026-07-20 — Enforced release-aware GitHub Container Registry retention

The main-only operator and simulator publication workflow retained the newest
20 complete immutable releases for each of `sockerless-admin`,
`sockerless-simulator-aws`, `sockerless-simulator-gcp`, and
`sockerless-simulator-azure`. Its release-aware selector kept each 12-character
source tag together with its `-amd64` and `-arm64` images and deleted obsolete,
untagged, or otherwise unrecognized package versions. The publication gate
locked the native runners, direct OCI architecture manifests, two-platform OCI
index, immutable tag grammar, complete package matrix, and retention invocation
into pull-request continuous integration and pre-commit validation.

## 2026-07-20 — Made the Shauth relying-party matrix hermetic

The Sockerless Admin and AWS, Google Cloud, and Microsoft Azure simulator
single-sign-on harness built each production frontend before compiling its Go
server. Clean continuous-integration runners therefore exercised the same
embedded interfaces as local runs instead of silently falling back to headless
binaries and returning `404` for `/ui/`. The matrix used the exact Shauth
verified-email revision and passed the real PostgreSQL, Ory Hydra, and Chromium
direct-entry, catalog-entry, shared-sign-on, identity, app-local landing, and
global-logout contract.

## 2026-07-20 - Real Shauth Relying-Party Contract

Sockerless Admin and the AWS, Google Cloud, and Microsoft Azure simulator
dashboards passed one real browser contract against PostgreSQL, Ory Hydra, and
Shauth. The matrix entered every application directly and through Shauth's app
catalog, signed in once, verified each application identity, initiated logout
from every relying party, observed global cross-application revocation, landed
exactly on the initiating application's public signed-out page, reloaded that
page without restarting authentication, and proved protected re-entry failed
closed. The test pinned the exact CI-green Shauth revision that served all
browser assets locally.

Admin registered its OIDC Front-Channel Logout route outside the local-session
boundary, preventing provider logout iframes from being redirected into an
interactive login page after the initiating session had already been revoked.
Admin and the shared simulator authentication module supported
`client_secret_post`, revoked local state before provider discovery failures,
required the OIDC Back-Channel Logout event claim to be the exact empty object,
and accepted explicit HTTP development coordinates only on loopback hosts.
Both front- and back-channel logout remained correlated to trusted issuer,
session, subject, and replay identifiers.

The dependency freshness gate also advanced `actions/setup-node` to its current
major release for the new browser job. The generated README status badges were
refreshed by the repository's sanctioned pre-push badge hook.

## 2026-07-19 - Polished Simulator Consoles and Global Admin Logout (`fix/simulator-console-ui`)

Sockerless Admin and the AWS, Google Cloud, and Microsoft Azure simulator
dashboards now shared a polished responsive shell with saturated accessible
light/dark palettes, consistent navigation, service-specific resource names,
keyboard focus treatment, and a screen-reader skip link. The current compiled
Go servers passed real Chromium coverage across every dashboard and Admin's
live overview, component status, metrics, reload, containers, and operational
pages through the real Docker passthrough backend. Every bundle served the same
self-contained Sockerless browser mark and Admin no longer depended on an
external font host. The Admin harness removed
its synthetic HTTP backend, used collision-free ports, detected dead child
processes, and cleaned its process tree deterministically instead of silently
testing against another local service.

Sockerless Admin also became a complete Shauth logout participant. Its browser
sessions were tracked server-side, verified OIDC Back-Channel Logout tokens
revoked matching sessions by `sid` or `sub`, replayed `jti` values were
rejected, and the user logout control initiated RP-Initiated Logout through
Shauth's discovered `end_session_endpoint`.

The AWS, Google Cloud, and Microsoft Azure dashboards used the same first-party
OpenID Connect relying-party module rather than an infrastructure-specific
authentication proxy. Direct UI entry used authorization code + PKCE with
state and nonce validation; signed local sessions exposed identity and a POST
logout control; RP-Initiated Logout carried the ID-token hint; and signed OIDC
Back-Channel Logout revoked sessions by `sid` or `sub` with `jti` replay
rejection. Only UI, identity, and logout routes were protected. Every native
cloud API slice retained its existing authentication and wire behavior.

Every cloud and GitLab smoke image now copied the shared OpenID Connect module
required by the standalone simulator graphs. The Google Cloud Run and Azure
Container Apps GitLab images also carried the shared agent module required by
their backend graphs, and the legacy AWS GitLab image selected the intentional
headless simulator build. All affected images compiled successfully; the exact
Amazon Elastic Container Service continuous-integration image also passed all
15 real simulator/backend Docker lifecycle assertions. A pre-commit and
continuous-integration contract now rejects any smoke Dockerfile that loses a
required shared module or compiles a GitLab simulator with its browser bundle
absent.

## 2026-07-19 - Authenticated Simulator Dashboards and Truthful Release Validation

The AWS, Google Cloud, and Microsoft Azure simulator dashboards now use their
shared first-party OpenID Connect session coordinates for signed-in identity
and application logout. Their shared shell displays the authenticated operator
with accessible user details and a real logout control while leaving every
cloud API route unchanged. Image publication runs only after a push to `main` and emits
the immutable short-SHA manifest plus its explicit `-arm64` and `-amd64`
images.

The nightly fuzz harness now selects the intentional headless build, skips
nested Go modules until their own matrix entry, reports build or target failures
without calling them crashers, bounds per-target workers, and collects only new
minimized inputs. Root-context simulator image builds also exclude local
dependency and generated output directories. The previous workflow failures
were resolved at their shared build, resource, and artifact boundaries rather
than being recorded as nonexistent parser crashes.

The repository-wide core test also reconciled the Go workspace checksum set
with the current transitive module graph, so the required gate leaves a clean
worktree on subsequent runs.

## 2026-07-19 - Amazon ECS A-Record Service-Registry Fidelity (`fix/ecs-a-record-service-registry-validation`)

The AWS simulator now validates Amazon Elastic Container Service service-registry port coordinates against the registered AWS Cloud Map DNS record type. A-record services reject `containerPort` or `port` with the same invalid-parameter contract as Amazon ECS, while portless registrations preserve task ENI discovery. Focused official AWS SDK coverage creates the real Cloud Map A-record registry, proves the rejected port-bearing request, and proves the valid portless request.

## 2026-07-18 - Cloud-Independent API-Only Simulator Runtime Contract (`feat/api-only-runtime-capability`)

The Amazon Web Services, Google Cloud, and Microsoft Azure simulators now exposed a common `/health` capability document with the configured runtime and a `workloadExecution` flag. `SIM_RUNTIME=process` remained a generic API-only simulator coordinate rather than a deployment-platform mode: storage, queues, eventing, audit, and control-plane slices continued to use their real API implementations, while callers could reliably discover that container workloads were unavailable.

The Microsoft Azure Container Instances slice no longer fabricated a successful running group in API-only mode. It returned a documented deployment failure before persisting a workload that it could not execute, matching the simulator's explicit runtime capability rather than presenting synthetic state. Focused shared-server health tests and a no-user-interface Azure Container Instances process-runtime test validated the contract.

The AWS RDS official SDK test module now used the current client release required by the repository freshness gate, keeping the simulator's required CI dependency graph current.

## 2026-07-18 - Operator Console Liveness (`fix/admin-health-liveness`)

Sockerless Admin served `GET /healthz` as a small unauthenticated liveness endpoint. Amazon Elastic Container Service and Shauth managed-app monitoring could therefore distinguish a live operator console from protected user-interface routes without attempting to authenticate a health probe. The browser console, administration API, Shauth authorization-code flow, and logout routes remained protected by the existing Shauth middleware. Focused operator-console tests verified the exact successful liveness response.

## 2026-07-17 - Immutable Sockerless Operator and Simulator Images (`feat/publish-sockerless-admin-image`)

Sockerless now published fully baked Amazon Elastic Container Service-ready images for the Shauth-capable operator console and all three cloud simulators. The Amazon Web Services, Google Cloud, and Microsoft Azure simulator images embedded their existing production web interfaces rather than compiling with `noui`; each continued to serve the same protocol-faithful cloud API and UI from its real binary. The operator image embedded the production Admin interface and ran as an unprivileged user.

The release workflow built every image natively on ARM64 and AMD64 runners, published `:<short-sha>-arm64` and `:<short-sha>-amd64`, and composed only `:<short-sha>` as the multi-architecture manifest. It emitted no mutable branch, semantic-version, or `latest` tags. Local ARM64 release-image builds passed for all four images. The three simulator images started in their documented API-only coordinate and served both `/health` and `/ui/`; the Admin image completed its Go tests, production web build, and startup check.

The required dependency-freshness gate also found an Amazon S3 service-client release and a Google API release across seven independently resolved backend and dispatcher module graphs. All graphs now use their current releases with reconciled transitive dependencies. The complete freshness gate and the affected Amazon Elastic Container Service, AWS Lambda, Google Cloud Run, Google Cloud Run Functions, common-library, and runner-dispatcher suites passed.

The same gate then found the matching Amazon S3, Smithy, and Google API drift in the AWS and Google Cloud simulator SDK graphs. Those official-client graphs were refreshed, and both complete SDK suites passed against their real simulator servers.

The simulator lint bootstrap now retries transient golangci-lint download transport errors with explicit connection and total-time limits, while `pipefail` preserves a real installer failure. This prevented a transient TLS reset from being reported as a source-lint defect.

## 2026-07-16 - Shauth Operator Sign-In and Simulator Quality Gates (`feat/shauth-operator-console`)

The Sockerless operator console gained optional Shauth OpenID Connect authorization-code sign-in with discovery, PKCE, nonce, state, signed HttpOnly sessions, audience validation, role enforcement, identity display, accessible avatar semantics, and logout. It guarded only the browser console and its administration API; the AWS, Google Cloud, and Azure simulator endpoints retained their native cloud protocols without browser-auth middleware.

The simulator dead-code gate now preserved analyzer diagnostics instead of exiting silently. The reported Azure failure identified and reconciled the simulator's standalone Go module graph after the SQLite shared-module refresh. The AWS, Google Cloud, and Azure dead-code scans and Azure no-UI module suite passed after that reconciliation.

## 2026-07-15 - Standalone Bleephub and Bleeplab Extraction (`chore/extract-bleep-products`)

Bleephub and Bleeplab moved into the independent `e6qu/bleephub` and `e6qu/bleeplab` repositories without retaining Sockerless commit history. Each repository retained exactly one root commit authored by the e6qu noreply identity. Bleephub now owns its Go server layout, web application, SSH gateway, dqlite node, Terraform module, tests, and official GitHub Actions runner consumer harness. Bleeplab now owns its server, user interface, tests, and official GitLab Runner consumer harness.

Sockerless removed the product implementations, user-interface packages, Terraform module, product workflows, administration wiring, stale local paths, and obsolete build artifacts. Documentation now treats both products as external consumers. The Bleephub runner harness builds its own product image and the real Sockerless simulator/backend binaries from a named build context; its spawned-runner image uses that same loaded harness image. The Bleeplab runner harness follows the same real consumer model. Terragrunt configuration in `e6qu/infra` pins the standalone Bleephub Terraform module root commit.

## 2026-07-14 - Targeted Main Validation and Standalone Cloud Run Builds (`fix-main-ci-trigger`)

Azure Key Vault purge now modeled the documented accepted long-running-operation form. The simulator deleted the recoverable vault, returned `202 Accepted` with an absolute Location operation URI, and served a terminal zero-length `200 OK` at that URI. The terminal poll URI is explicitly allowed by the Swagger conformance ratchet because the documented Location target has no upstream Swagger path. This let the current generated AzureRM client complete `VaultsPurgeDeletedThenPoll` without attempting to poll an already removed deleted-vault resource. Focused Azure Key Vault SDK coverage and a real Dockerized AzureRM apply, idempotency plan, and destroy run passed.

The Azure Container Registry simulator now returned `properties.roleAssignmentMode` on registry reads and preserved an explicit requested setting. When the request omitted it, the simulator returned Azure's `LegacyRegistryPermissions` default, matching Microsoft.ContainerRegistry and preventing current AzureRM Terraform from proposing a perpetual in-place registry update. The command-line interface contract asserted that default. The macOS nested Azure Terraform harness also loaded its Buildx-built test image into Docker's image store before it started the inner test. A complete Dockerized AzureRM apply, idempotency plan, and destroy run passed.

The fully baked Bleephub release image retried each Ubuntu dependency installation transaction from a freshly downloaded package index. This made the native ARM64 build resilient to an Ubuntu archive publication race where a just-replaced package returned `404` after the prior index had selected it. A complete local ARM64 release-image build passed through both dqlite installation stages and final image export.

CI kept the complete validation matrix on pull requests and moved post-merge `main` work into a dedicated Bleephub publication workflow. Every merge built native AMD64 and ARM64 images on their matching runners, published them as `ghcr.io/e6qu/sockerless-bleephub:<short-sha>-amd64` and `:<short-sha>-arm64`, and composed `:<short-sha>` as their multi-architecture manifest. It published no mutable `main` or `latest` tag, retained only the newest 20 short-SHA releases and their architecture variants, and did not restart simulator, browser, build, Terraform, or runner checks after merge.

Each native architecture tag now published a direct OCI image manifest without a Buildx provenance index. This made its referenced architecture manifest anonymously retrievable from GitHub Container Registry, which Amazon ECS on AWS Fargate required before it could pull the ARM64 or AMD64 member of the public multi-architecture release.

Closed BUG-2591 by upgrading stale Amazon Cloud Map, AWS Lambda, and Amazon Simple Systems Manager Go service clients in the Amazon Elastic Container Service backend, AWS Lambda backend, Bleephub wake function, and AWS simulator software development kit module. The affected backend, wake, and simulator software development kit suites passed against the updated clients, and repository dependency freshness passed.

Closed BUG-2592 by making Bleephub site administrators authoritative for repository authorization. An external GitHub administrator with a registered SSH key could now read, push, and administer organization-owned repositories through the same Git Smart HTTP and SSH checks as every other Git client; focused SSH transport coverage and the complete Bleephub suite passed.

Retention resolved the repository owner's GitHub account type before calling the GitHub Packages API, so the user-owned `e6qu` namespace used `/users/` while organization namespaces used `/orgs/`.

The Bleephub idle controller now switched the public route back to wake and set the application plus every dqlite voter to zero in one Amazon Elastic Container Service control-plane pass. Amazon Elastic Container Service completed the real connection drain asynchronously, so the Lambda did not time out and leave a partial quorum running.

The Bleephub Terraform module now published a cache-controlled, non-sensitive startup document from a dedicated Amazon Simple Storage Service bucket through an explicit Amazon API Gateway route. The wake Lambda retained only capacity control and token-protected administrator status JSON, while the document visibly tracked startup, loaded the healthy Bleephub document without a browser refresh, and showed administrator Amazon Elastic Container Service counts plus direct Amazon CloudWatch Logs, Amazon CloudWatch idle-alarm, and Amazon ECS console links only after administrator-token authentication. The release workflow built the versioned startup ZIP as a GitHub Container Registry package and retained its newest 20 releases alongside the multi-architecture Bleephub images. Every authenticated and sign-in Bleephub page showed the immutable image version and publication timestamp embedded at release build time.

The wake module also used the current Amazon Lambda SDK release required by the dependency-freshness gate.

The Google Cloud dependency refresh also reconciled the Cloud Run and Cloud Run Functions module graphs under `GOWORK=off`. The release-matrix no-UI binaries now build with their standalone module metadata instead of requiring a workspace-mediated dependency selection.

## 2026-07-14 - Bleephub Terraform Module Relocation (`feat/bleephub-terraform-module`)

The reusable Bleephub Amazon Elastic Container Service on AWS Fargate module moved from the generic Terraform module tree to `bleephub/terraform`, together with its Amazon Web Services simulator apply/destroy coverage and pre-built wake-listener source. The wake build script and relocated test resolved repository paths from the new module location. The superseded checked-in Terraform root was removed so the private `e6qu/infra` Terragrunt repository became the single production environment owner. The module README documented its required inputs, hosted origins, output contract, and secret-safe GitHub OAuth configuration.

## 2026-07-13 - Bleephub Amazon Elastic Container Service on AWS Fargate Deployment (`feat/bleephub-hosted-compute-network-onboarding`)

Bleephub deployed in a dedicated eu-west-1 Amazon Elastic Container Service on AWS Fargate stack rather than the separate EDD infrastructure. The reusable Terraform module provisioned private application networking with fck-nat, an Amazon Simple Storage Service gateway endpoint, encrypted Amazon Simple Storage Service git/object buckets, Amazon Elastic File System-backed native dqlite voters, an internal Network Load Balancer, Amazon API Gateway public wake routing, an administrator origin, and a hardened SSH Git gateway. The fully baked ARM64 release image performed no build work at task start.

GitHub OAuth, administrator-provisioned local users, the e6qu-org administrator/developer mapping, Git Smart HTTP, and SSH public-key Git were wired through production configuration. Live verification created a repository, registered an ephemeral SSH key, pushed and cloned over SSH, cloned over HTTPS, used the official GitHub command-line interface against the live server, and confirmed the healthy UI/API routes.

The idle controller armed a five-minute API-request alarm after traffic, safely quiesced the Amazon CloudWatch alarm before shutting down, scaled application and dqlite services to zero, and restored the full quorum on a subsequent cold wake. A live cold wake restored all three dqlite voters and the application before returning successful health responses. The git bucket had versioning suspended and a noncurrent-version lifecycle rule to prevent retained historical object costs.

The production browser harness now started the real SSH Git listener with a disposable host key and advertised a port-aware `ssh://` clone coordinate whenever the configured SSH host included a non-default port. This preserved GitHub-style SCP coordinates for production port 22 while giving Playwright a valid local transport. The empty-repository page therefore rendered its real SSH selector under test. The embedded user interface also served a saturated Bleephub SVG favicon instead of returning the single-page application shell for the browser icon request. Focused real Chrome verification created a repository through the public API and confirmed both the SSH selector and favicon link; the complete Bleephub Go suite passed in 221 seconds.

The native ARM64 core continuous-integration job now applied an explicit eight-minute deadline to the complete Bleephub test package. The prior shared five-minute deadline expired while the final webhook timeout test was running after every preceding Bleephub test had passed; the package retained its complete coverage and the other core packages retained their five-minute deadlines.

The repository dependency-freshness gate found stale cloud and supporting Go module pins before the timeout fix could be pushed. The affected Bleephub, cloud-backend, simulator, runner-dispatcher, agent, and command modules were updated to the current versions required by that gate. Dependency freshness then passed and the complete Bleephub suite passed in 218 seconds with the updated graph.

The primary continuous-integration workflow now ran for pull requests targeting `main` and for every push to `main`. A merged change therefore received the same independent post-merge validation as its pull request rather than leaving the protected branch without a run.

The required freshness gate also surfaced newly published Google Cloud and supporting module releases before the CI repair could be pushed. The affected modules, including the Google Cloud common backend's Cloud Build and Cloud Run clients, were upgraded and validated by the same gate.

## 2026-07-12 - GitHub Marketplace Publisher and Buyer Product (`feat/bleephub-ui-api-completeness-audit`)

Closed BUG-2548 through BUG-2560 and removed GitHub Marketplace from BUG-2523. GitHub App and OAuth App owners created durable draft/published listings, dedicated signed webhooks, delivery history, and free, flat-rate, or per-unit monthly/annual plans through authenticated settings. Publisher REST plan and account reads required the owning GitHub App's JSON Web Token or OAuth App's Basic client credentials, kept unrelated publishers isolated, preserved GitHub's production and `stubbed` shapes, returned empty collections rather than null, and excluded confidential webhook configuration from public buyer listings.

Authenticated buyers browsed a GitHub-organized Marketplace, searched saturated app cards, compared plans, selected a personal or administered organization account, started trials, completed a GitHub App installation or OAuth App installation-URL handoff, and managed upgrades, downgrades, and cancellations. Upgrades began immediately; paid downgrades and cancellations waited for the billing boundary; free/trial cancellations began immediately; and purchased, changed, cancelled, and ping events used the listing-owned webhook. Subscription identity included listing, account type, and account ID, preserving multiple app purchases and colliding User/Organization numeric identifiers.

Marketplace listings, plans, webhook configuration/deliveries, subscriptions, pending changes, and installations survived SQLite reload. New subscription plus GitHub App installation creation committed in one SQLite transaction before either webhook began, and real closed-storage coverage proved that failure left no memory or installation residue. Plan/listing edits and deletion enforced active-purchase and published-plan invariants. The obsolete `/internal/marketplace/purchases` route and synthetic global free plan were removed.

The routed buyer directory/detail and GitHub App publisher editor retained GitHub's hierarchy while using a candy-saturated purple, blue, cyan, pink, green, and gold palette in both themes. Real Chromium also exposed and fixed the GitHub App dialog's opaque manual-redirect mistake: App Manifest creation now followed the real same-origin redirect and converted the code from its final URL. Expected absent publisher listings used a nullable `200` browser adapter instead of console-error `404` probes.

The complete Bleephub Go suite passed in 216 seconds; the user-interface suite passed 48 files / 334 tests; TypeScript, production build, and the unused-export gate passed with the tracked current-`knip` deprecation only; the complete real-Chromium suite passed 31/31; the complete official `go-github` suite passed; and the Dockerized official `gh` command-line interface harness passed 136/136. The complete all-files pre-commit gate also passed. Visual inspection confirmed distinct, legible light and dark Marketplace surfaces and the saturated discovery treatment. Cleanup removed 22 GiB of disposable Go build cache, temporary hook/package caches, 21 stale Amazon Elastic Container Service simulator task containers, and unused images without touching active services or volumes, increasing local free space from 31 GiB to 54 GiB.

## 2026-07-12 - GitHub CodeQL Producer and Code Security (`feat/bleephub-ui-api-completeness-audit`)

Closed BUG-2535 through BUG-2540 and removed CodeQL database production from BUG-2523. Bleephub accepted the official GitHub CodeQL Action uploads-host request with a raw ZIP body, language, name, and real commit object ID; validated safe finalized CodeQL database or legacy database bundles with a language dataset; persisted archives in the object byte store; and removed the arbitrary internal base64 seed route. Public list, get, download, and delete behavior used GitHub's `contents` read/write permissions, honored repository-selected GitHub App installation tokens, protected private database and variant-analysis bytes, and returned GitHub-compatible download metadata.

Database replacement used immutable content-addressed object keys and preserved the prior metadata and bytes across object-store, SQLite, or cleanup failure. SARIF ingestion required a fully qualified ref and real repository commit, accepted GitHub Actions installation credentials with `security_events` permissions, preserved UTF-8 payloads, and created durable analyses even for valid zero-finding runs. The official producer and browser therefore shared truthful git coordinates instead of fabricated branches or all-zero object IDs.

The repository Security page was reorganized around GitHub-style Code scanning navigation, finding filters and detail, CodeQL database management, analyses, and SARIF upload. Its light and dark themes retained GitHub surface hierarchy while using saturated blue, cyan, purple, pink, and gold treatments. The account token hero also moved onto valid shared background, status, and elevation tokens, closing BUG-2541 and the prior CI gradient failure.

Closed BUG-2542 through BUG-2547 while making the browser and hygiene proof strict: the Code Security scenario used unambiguous real-commit locators, selected dark mode through the user menu, waited for the accepted producer response and rendered analysis, the user-interface API module no longer exported an unused single-alert helper, and SARIF ingestion preserved every run in multi-language or multi-configuration documents. The complete real-Chromium suite passed 30/30, the user-interface suite passed 46 files / 330 tests, TypeScript and `knip` passed, the complete Bleephub Go suite passed in 204 seconds, the official `go-github` suite passed, and the Dockerized official `gh` command-line interface harness passed 130/130.

## 2026-07-12 - Fine-Grained Personal Access Tokens (`feat/bleephub-ui-api-completeness-audit`)

Closed BUG-2531 and BUG-2532 and removed fine-grained personal access tokens from BUG-2523. Authenticated account settings created durable `github_pat_` credentials for a user or active organization membership, constrained them to one resource owner, all/selected/no repositories, explicit repository and organization permissions, and an optional expiration, and displayed the secret exactly once. The polished GitHub-organized account page listed active, pending, revoked, and expired credentials, exposed organization-owner approval decisions, deleted owned credentials, and retained saturated, legible light/dark presentation.

Runtime authentication distinguished classic and fine-grained credentials. Pending, expired, revoked, cross-owner, unselected-repository, and ungranted-permission access was denied; repository inventories omitted inaccessible private resources while retaining GitHub's public-resource behavior; deletion removed authentication and associated request/grant state; API insights retained the fine-grained token identity; and SQLite reload preserved the complete credential contract. Organization request and grant REST administration became GitHub App-only with the official `organization_personal_access_token_requests` and `organization_personal_access_tokens` permission names for targeted installation and user access tokens.

Closed BUG-2534 by extending the repository secret-scanning and push-protection detector to generated `github_pat_` credentials and Bleephub's generated classic credential length. Committing a live fine-grained token now creates the same GitHub personal access token alert class instead of bypassing detection.

Official `go-github` coverage created the credential through the browser producer, minted a real GitHub App installation token, listed and approved the request, and listed the resulting grant. The Dockerized official `gh` command-line interface harness created and authenticated a one-time credential and passed as part of the branch's 130/130 cases. Account component tests, the passing real-Chromium scenario, the complete Bleephub Go suite, 46 user-interface test files / 330 tests, typecheck, and the production build covered the implementation.

Closed incidental BUG-2533 after the required browser check exposed stale routed release-edit state. Saving an edit now reconciled the detail query, exited editor state, and kept uploaded assets available for download and deletion; focused component coverage preserved that transition.

## 2026-07-12 - Retained GitHub Classroom Product (`feat/bleephub-ui-api-completeness-audit`)

Closed BUG-2527 through BUG-2530 and removed GitHub Classroom from BUG-2523. The six official read-only GitHub Classroom REST endpoints became organization-admin scoped and were exercised through current `go-github` types and the official `gh` command-line interface. The obsolete Classroom operator seed routes were removed.

Bleephub retained the browser product with saturated GitHub-adapted light/dark organization. Organization administrators created, renamed, archived, and deleted classrooms; managed linked or identifier-only rosters; created individual and group assignments with deadlines, repository visibility, student permissions, team limits, feedback pull requests, and command-based autograding; and exported or imported lossless transition bundles after repository migration. Invite URLs routed into the product and authentication preserved the requested destination.

Acceptance copied the real starter git tree into an organization-owned repository, granted each student access, serialized concurrent decisions, enforced group capacity without partial roster claims, created the configured Feedback branch and pull request, installed a real GitHub Actions workflow, and recorded the baseline commit. Classroom counters and grade exports derived subsequent commits, deadline submission state, completed job results, and available/awarded points from real repository and Actions state instead of management input. Classroom metadata, rosters, autograding configuration, acceptances, and transition identity survived SQLite reload.

The completeness audit also closed BUG-2520 through BUG-2522 and BUG-2524 through BUG-2526. Bleephub's shared light/dark visual system retained GitHub/Primer surface and semantic hierarchy while adding saturated blue, cyan, purple, pink, gold, and green brand/state treatments. Repository context chrome became full-width and organized around GitHub's primary tabs, content shortcuts, administrative overflow, real Watch/Star toggles, and an owner-selecting Fork workflow backed by the public REST API. An authenticated `/ui-data` viewer-state read prevented expected public existence-check `404` responses from becoming browser resource errors while mutations stayed public. Browser and repository-social tests became independently provisioned and route-aware.

The parity specification was reconciled against the implementation. It removed already-fixed GitHub App selection, installation webhook, and App-hook gaps; documented the REST/state/event/UI proof boundary; and identified the remaining GraphQL-schema, REST-semantic, page-level UI, and external-ingress work. It identified GitHub Marketplace and hosted-compute network settings as the two remaining operator-ingress domains; the Marketplace section above recorded the completed public replacement, leaving hosted-compute onboarding open.

The release-provider compatibility pass also closed BUG-2518 and BUG-2519 after CI exercised the new workflows. The official GitHub software development kit release lifecycle established a real initial commit and `refs/heads/main` through GitHub's Git Database API before creating a release. The routed browser release scenario reused the exact uploaded asset buffer when asserting its displayed size, so it continued through authenticated download and deletion without a divergent hardcoded byte count.

## 2026-07-12 - Bleephub Release Provider Completeness (`feat/bleephub-ui-api-completeness-audit`)

This branch continued from merged #791 and audited Bleephub's UI routes against its implemented public GitHub API and real state. It identified the release provider as a complete class gap rather than a single missing screen.

Closed BUG-2512 by replacing the transient read-only release list with routed repository release workflows. `/ui/repos/{owner}/{repo}/releases`, `/releases/new`, and `/releases/{id}` now support deep links and browser history; create, edit, draft/pre-release state, delete, object-backed asset upload, authenticated asset download, and asset deletion all use the public GitHub Releases API. The Code view links into the routed manager instead of trapping release state in a local tab.

Closed BUG-2513 and BUG-2514 by making release identity repository-scoped and git-backed. Updates verify ownership before validation or mutation. Creation and tag-name changes resolve an existing real tag or resolve `target_commitish` and create a real lightweight tag, while duplicate releases and unresolved targets return validation errors without changing release or git state.

Closed BUG-2515 by deriving release webhook and GitHub Actions activity from real lifecycle transitions. Complete release payloads now carry `created`, `edited`, `published`, `unpublished`, `prereleased`, `released`, or `deleted`, with GitHub's draft workflow semantics. Closed incidental BUG-2516 by removing every remaining asynchronous workflow-discovery call from pull-request REST/GraphQL and repository-dispatch handlers, eliminating mutable go-git read/write races across the eventing class.

Closed incidental BUG-2517 by upgrading Bleephub's Markdown parser from `github.com/yuin/goldmark` 1.8.3 to current 1.8.4 after the required pre-push dependency-freshness gate detected the drift.

Validation in this branch included:

```bash
bun run --cwd ui/packages/bleephub typecheck
bun run --cwd ui/packages/bleephub test
bun run --cwd ui/packages/bleephub build
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'Test(Releases_|WebhookReleaseLifecycleActions)' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -race -tags noui ./bleephub -run 'Test(Releases_|WebhookReleaseLifecycleActions)' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -count=1
```

## 2026-07-12 - Bleephub GitHub Pages Branch Publication (`feat/bleephub-pages-branch-builds`)

This branch continued from merged #790, which made GitHub Actions artifact deployments publish real GitHub Pages sites from object storage.

Closed BUG-2506 by replacing permanent shape-only Pages build queues with real branch publication. `POST /api/v3/repos/{owner}/{repo}/pages/builds` now resolves the configured legacy source branch and `/` or `/docs` subtree from git, treats `.nojekyll` as already-built static output, rejects symbolic links, submodules, unsafe paths, empty sources, and content over 10 GB, writes a deterministic TAR archive through the same transactional S3-compatible publication path as workflow deployments, serves the result, and persists the actual commit, duration, terminal build/site state, custom-404 state, digest, size, and deployment record. Object replacement writes and validates the new durable object before deleting the prior publication and rolls back the new object if replacement cleanup fails.

Closed BUG-2507 by shipping and executing the real GitHub Pages generation runtime. The release image now contains Ruby, Bundler, `github-pages` 232, Jekyll 3.10.0, and the complete GitHub-supported plugin/theme graph behind `bleephub-pages-jekyll`. Branch builds without `.nojekyll` materialize the real git source in an isolated workspace, invoke Jekyll in safe production mode with repository identity, bound captured build output to 1 MiB, archive only regular generated files, and publish through the same object transaction. Malformed sites persist real terminal Jekyll errors and create no deployment. Unconditional integration coverage built the actual release image and proved Markdown/Liquid generation plus object-backed serving against real git and the Amazon Simple Storage Service simulator.

Closed BUG-2508 by routing smart HTTP pushes, Contents API commits, and Git Database branch-reference creates, updates, and deletes through one committed-reference event path. Every branch mutation now records repository activity, emits the push webhook, triggers GitHub Actions workflows, synchronizes matching pull requests, and automatically builds a configured legacy Pages source branch. The event consumers run in a race-safe order against the shared git store, and coverage proved automatic publication through both Contents API and Git Database writes.

Closed BUG-2509 by serializing workflow-run `actor` and `triggering_actor` fields through the complete GitHub simple-user representation rather than an abbreviated webhook-only shape. Closed BUG-2510 by resolving git storage through canonical repository IDs and metadata coordinates through `full_name`, so committed-reference processing and Pages source reads do not dereference optional expanded owner representations.

Closed incidental BUG-2511 by upgrading Bleephub's Markdown parser from `github.com/yuin/goldmark` 1.8.2 to current 1.8.3 after the required pre-push dependency-freshness gate detected the drift.

Validation in this branch included:

```bash
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'Test(StaticPagesBranchArtifactValidation|PagesBuildsCRUD|PagesCreateUpdateShape)' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -race -tags noui ./bleephub -run 'Test(StaticPagesBranchArtifactValidation|PagesBuildsCRUD|PagesCreateUpdateShape)' -count=1
docker buildx build --load -f bleephub/Dockerfile.release -t sockerless-bleephub-pages-test .
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'TestPagesJekyllBuildPublishesGeneratedSite' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'Pages|pages' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -race -tags noui ./bleephub -run 'Test(PagesBuildsCRUD|Dependabot_OrgAlerts|EnterpriseDependabotAlerts|SecretScanning_OrgAlerts|SecretScanning_PushProtectionBypasses|SecretScanning_PushProtectionBlocksGitDatabaseRefBeforeMutation)$' -count=1
```

## 2026-07-12 - Bleephub GitHub Pages Artifact Fidelity (`feat/bleephub-fidelity-sweep-next`)

This branch continued from merged #788, which hardened persisted repository ownership, Git provider and user-interface behavior, GitHub Apps authorization, GitHub Actions execution and runner contracts, container packages, and Projects v2 ownership.

Closed BUG-2502 by making GitHub Pages deployments consume real artifact bytes before reporting success. `POST /api/v3/repos/{owner}/{repo}/pages/deployments` now retrieves either the supplied artifact URL or the repository-owned GitHub Actions artifact, reads object-backed artifacts from S3-compatible object storage, rejects unreadable artifacts and metadata/byte-size mismatches without changing Pages state, and records the deployed byte count and SHA-256 digest. Coverage exercised both accepted inputs through Bleephub's real artifact-download data plane and object storage.

Closed BUG-2503 by completing the publication operation behind deployment success. Bleephub now validates the official GitHub Actions ZIP containing `artifact.tar` plus direct ZIP, TAR, and gzip-compressed TAR inputs; rejects links, path traversal, empty archives, and content over GitHub's absolute size limit; stores immutable published archives in S3-compatible object storage; advertises a usable Bleephub Pages URL; serves index files, clean URLs, static assets, HEAD responses, and custom `404.html`; gates private sites on repository access; reclaims superseded publication objects; and removes published bytes before Pages or repository deletion.

Closed BUG-2504 by validating GitHub Actions workflow identity before publication. The Pages deployment endpoint now verifies the Bleephub OpenID Connect token's RS256 signature, key identifier, issuer, audience, validity window, repository and repository identifier, environment, build SHA, and configured source branch. Malformed, altered, expired, cross-repository, cross-environment, wrong-ref, wrong-build, and wrong-audience tokens fail before artifact retrieval or state mutation.

Closed BUG-2505 by adding GitHub's distinct `pages` fine-grained repository permission to the authorization model. Pages writes require `pages: write`; private Pages reads require `pages: read`; classic `repo` scope continues to cover Pages; and repository `administration` permission no longer grants Pages access.

Validation in this branch included:

```bash
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'Test(PagesDeployments_CreateStatusCancel|PersistenceReload_DeleteRepoLeavesNoResidue)' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'Test(PagesDeployments_CreateStatusCancel|PagesArtifactValidationRejectsUnsafeAndEmptyArchives|PagesPermissionIsDistinctFromAdministration|PagesHealthCheck|RegisteredAPIv3RoutesExistInGitHubSpec|FuzzRoutePatternsMatchRegisteredRoutes|PersistenceReload_DeleteRepoLeavesNoResidue)' -count=1
pre-commit run --all-files
```

## 2026-07-11 - Bleephub Public State Fidelity (`feat/bleephub-public-state-fidelity`)

This branch continued from merged #787, which moved CodeQL variant-analysis query-pack tarballs to object storage and made runner-log object-store failures preserve live process state.

Closed BUG-2491 by making persisted repository ownership strict. Repository reload now requires valid `owner_type` and `owner_id`, loads organizations before repositories so organization-owned repositories validate against real organization state, and fails loudly when persisted owner data is missing or inconsistent. Public repository listing and event paths no longer treat empty owner types as user repositories.

Closed BUG-2492 by removing the internal runner-submission image fallback. `/internal/exec/submit` and `/internal/exec/workflow` now require either an explicit `image` or `hostMode`, and tests that intend container execution pass `alpine:latest` explicitly instead of relying on hidden server-side defaulting.

Closed BUG-2493 by moving container-package coverage onto the real GitHub Container Registry-compatible data plane. Container package fixtures now publish blobs and manifests through the OCI/Docker Registry HTTP API v2 routes, package REST tests observe the resulting manifest/layer files, source coverage rejects new internal container-package seed calls, and `/internal/packages` rejects `container` package creation instead of leaving a parallel operator-only publish path.

Closed BUG-2494 by making Projects v2 GraphQL project creation resolve owners strictly. `createProjectV2` now requires the supplied `ownerId` to match a real user or organization GitHub node ID, returns a GraphQL error for unknown owner IDs, and does not mutate project state when owner resolution fails.

Closed BUG-2495 by removing the hidden execution-image default from public GitHub Actions workflow trigger and rerun paths. Push/event-triggered workflows, full-run reruns, failed-job reruns, and single-job reruns now preserve host-mode runner messages when the workflow YAML has no `container:` declaration, matching GitHub's runner contract instead of injecting `alpine:latest`.

Closed BUG-2496 by removing alternate base64 decoders from the GitHub Actions runner OAuth public-key path. Runner registration now accepts only the Azure DevOps/GitHub Actions runner protocol's standard base64 RSA modulus and exponent fields, and URL-safe or raw base64 variants fail loudly instead of creating a second public-key wire format.

Closed BUG-2497 by making GitHub Actions workflow parsing reject missing or invalid runner labels for normal jobs. Normal jobs now require `runs-on` to be a non-empty string or non-empty string list, reusable-workflow call jobs remain valid without runner labels, and job-list responses no longer invent `ubuntu-latest` when a directly seeded job lacks a definition.

Closed BUG-2498 by making public repository commit listing distinguish empty or broken git state from a successful empty history. `GET /api/v3/repos/{owner}/{repo}/commits` now returns GitHub's `409` empty-repository response when the default branch has no ref, returns a fail-loud service error when git storage cannot be opened or walked, and no longer reports `200 []` for a repository whose git history is unavailable.

Closed BUG-2499 by making Bleephub repository UI pages consume the new public commit-listing semantics faithfully. The UI now treats only GitHub's exact empty-repository `409` response as an empty commit history for display, while every other commit-listing conflict or storage failure still surfaces as an error.

Closed BUG-2500 / GitHub issue #789 by making organization repository creation honor GitHub App installation-token permissions. `POST /api/v3/orgs/{org}/repos` now authorizes installation tokens by target organization and `administration: write`, while installation tokens without that grant still receive `Resource not accessible by integration` and human tokens still require organization membership.

Closed BUG-2501 by separating Bleephub repository-page empty-history rendering from the public GitHub REST commit-listing contract. `GET /api/v3/repos/{owner}/{repo}/commits` still returns GitHub's `409` empty-repository response, while the authenticated `/ui-data/repos/{owner}/{repo}/commits` route maps only that exact empty git state to `200 []` for the browser UI. Storage and object failures still return service errors, and repository pages no longer emit strict browser-console resource errors for handled empty repositories.

Validation in this branch included:

```bash
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'Test(PersistenceReload_(OwnerAndCountersAndState|OrganizationRepositoryOwnerIsValidated|RepositoryMissingOwner(Type|ID)FailsLoud)|InternalSubmit(Job|Workflow)RequiresExplicitImageOrHostMode)' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'Test(ConcurrencyGroups_(RepoAndRunEndpoints|CompletedRunReleasesLease)|SubmitWorkflow(RepoRefResolution|RejectsUnresolvedRepoRef)|Workflows_Dispatch|InternalSubmit(Job|Workflow)RequiresExplicitImageOrHostMode)' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'TestPackages|TestContainerRegistry|TestLivePackages' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'TestProjectsV2GraphQL_(CreateProjectRequiresResolvedOwner|CreateProjectUsesResolvedUserAndOrganizationOwners|FieldValueKinds|ProjectLevelConnections)' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'Test(RerunKeepsRunIDAndBumpsAttempt|RerunFailedJobsCarriesSuccesses|RerunWorkflowJob_NewAttemptCarriesOtherJobs|Workflows_Dispatch|Workflows_DispatchUsesHostModeWhenWorkflowHasNoContainer)' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'Test(AgentRSAPublicKeyRequiresProtocolStandardBase64|OAuthToken|OAuthTokenRejectsMissingAssertion|OAuthTokenRejectsUnknownClient|RegistrationTokenRandom|GenerateJITConfig|RemoveToken)' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'Test(ActionsPendingDeploymentReviewFlow|WorkflowParseRequiresValidRunsOnForNormalJobs|WorkflowParseReusableWorkflowJobDoesNotRequireRunsOn|WorkflowParse(ContainerAsString|ContainerAsObject|Env|JobOutputs|StrategyFailFast))' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'Test(ListCommitsEmptyRepositoryFailsLoud|GetSingleCommit|CommitBranchesWhereHead|CommitPulls|CommitArchiveDownload)' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'Test(ListCommitsEmptyRepositoryFailsLoud|UIListCommitsEmptyRepositoryReturnsEmptyHistory|GetSingleCommit|CommitBranchesWhereHead|CommitPulls|CommitArchiveDownload)' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'Test(InstallationTokenCreatesOrganizationRepositoryWithAdministrationPermission|InstallationTokenCreateOrganizationRepositoryRequiresAdministrationWrite|InstallationTokenDownscoping|CreateOrgRepo)' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -count=1
bun run --cwd ui/packages/bleephub test src/__tests__/api.test.ts
bun run --cwd ui/packages/bleephub test
bun run --cwd ui/packages/bleephub typecheck
cd ui && bunx turbo run build --filter="*bleephub*"
pre-commit run --all-files
```

## 2026-07-11 - Bleephub CodeQL Variant-Analysis Query Pack Objects (`feat/bleephub-codeql-variant-query-pack-objects`)

This branch continued from merged #786, which moved more Bleephub service bytes to object storage and hardened public GitHub-compatible ingestion, deletion, and official-client coverage.

Closed BUG-2489 by moving CodeQL variant-analysis query-pack tarballs out of SQLite metadata and into the configured object byte store. Variant-analysis rows now persist controller, actor, language, target, status, query-pack size, and object-key metadata; public query-pack downloads read the object store; persistent stores fail loudly without `BLEEPHUB_OBJECT_S3_BUCKET`; and controller-repository deletion purges query-pack objects before deleting repository metadata.

Closed BUG-2490 by making GitHub Actions runner-log upload and run-log deletion complete required object-store writes/deletes before mutating in-memory log, console, or timeline state. Fail-loud object-store errors now preserve the previously visible process state instead of leaving live state diverged from durable object storage.

Validation in this branch included:

```bash
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'Test(LogfilesUpload_(WritesObjectStore|ObjectStoreFailurePreservesState|AppendsBlocks|CapsAtFourMiBWithMarker)|JobLogs_ReadsUploadedLogFilesFromObjectStore|RunLogsDelete_ObjectStoreFailurePreservesState|ActionsRuns_DeleteLogs)' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'Test(CodeQLVariantAnalyses_|PersistentServerStorageRequiresDurableGitAndObjectBytes|AgentsCodeScanPersistenceReload|PersistenceReload_(DeleteRepoLeavesNoResidue|RenameRepoMovesRepoScopedMetadata|TransferRepoMovesRepoScopedMetadata))' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -count=1
bash -n scripts/bleephub-local-dev.sh
git diff --check
pre-commit run --all-files
```

## 2026-07-10 - Bleephub Object-Backed Service Bytes (`feat/bleephub-object-backed-service-bytes`)

This branch continued from merged #785, which cleaned Codespace runtime/workspace state during repository deletion, hardened the AWS SDK simulator CI shard, and made persisted Bleephub require object-backed GitHub Actions artifacts, dependency caches, and runner logs.

Closed BUG-2471 by extending the object-backed byte-storage contract to release assets and GitHub Packages. Release asset uploads, package version files, and GitHub Container Registry blobs now write through the configured S3-compatible object store when it is present; SQLite stores metadata and object keys. Persisted startup and local development documentation now describe `BLEEPHUB_OBJECT_S3_BUCKET` as the required store for all durable service bytes, and release asset object-delete failures surface through the API and repository deletion path instead of being ignored.

Closed BUG-2472 by making public GitHub Packages file downloads read object-backed package file bytes. The metadata/listing path and the byte-serving path now use the same object storage source, so advertised package file REST URLs work for object-backed service bytes instead of looking only for local filesystem paths.

Closed BUG-2473 by making repository deletion fail loudly on required git-storage cleanup failures. Bleephub now purges filesystem or S3-backed git storage before deleting repository metadata; if S3 git-prefix cleanup cannot be resolved or completed, the delete returns an error and preserves the repository record and git storage index instead of logging and orphaning git objects.

Closed BUG-2486 by making repository deletion purge repository-owned GitHub Packages file bytes before deleting repository metadata. Object-backed and local package file bytes now go through the required cleanup path, so package byte cleanup failures surface as repository-delete errors instead of leaving durable package objects behind after package metadata is gone.

Closed BUG-2487 by moving GitHub CodeQL database archive bytes out of SQLite metadata and into the configured object byte store. CodeQL database rows now persist metadata, size, and object keys; public archive downloads read the object store; database deletion removes the object before deleting metadata; and repository deletion purges repository-owned CodeQL database archive objects before deleting repository metadata.

Closed BUG-2488 by moving artifact attestation Sigstore bundle bytes out of SQLite metadata and into the configured object byte store. Attestation rows now persist repository linkage, subject digests, predicate type, initiator, timestamps, and object keys; repository, organization, and user attestation list endpoints read bundle JSON from object storage; public attestation deletion removes object bytes before metadata; and repository deletion purges repository-owned attestation bundle objects before deleting repository metadata.

Closed BUG-2474 by upgrading stale AWS software development kit service modules found by the pre-push dependency freshness gate. The Amazon Elastic Container Service backend, AWS Lambda backend, and AWS simulator software development kit tests now use the latest published CloudWatch, Amazon Elastic Compute Cloud, and AWS Lambda service module versions required by the gate.

Closed BUG-2475 by moving the Bleephub go-github software development kit harness's organization provisioning onto GitHub Enterprise Server's public admin organization API. The SDK tests no longer create organizations through `/internal/orgs`, and source coverage rejects that operator-only route in the official-client harness.

Closed BUG-2476 by moving Bleephub public GitHub REST test organization setup onto GitHub Enterprise Server's public admin organization API. Public feature tests now use a shared `/api/v3/admin/organizations` helper for prerequisite organizations, while the only remaining direct `/internal/orgs` organization-creation calls are explicit operator-management coverage; a source guard rejects new direct public-test setup calls to the operator route.

Closed BUG-2477 by moving Bleephub public code scanning alert setup onto GitHub's public SARIF upload route. The shared code scanning alert helper now uploads SARIF to `/api/v3/repos/{owner}/{repo}/code-scanning/sarifs`, live-shape and campaign coverage use that public ingestion path, and SARIF rule severity/description metadata now flows into persisted alert state for filtering and downstream features.

Closed BUG-2478 by making the Bleephub UI typecheck pre-commit hook rebuild `@sockerless/ui-core` declarations before checking Bleephub. The hook clears stale ignored incremental build state, emits the required declarations, and then runs Bleephub `tsc`, so cleaning generated `dist` output no longer leaves the hook dependent on manual repair.

Closed BUG-2479 by making Bleephub secret scanning derive alerts from real repository content. Contents API writes now scan the new commit for supported provider secret patterns, Git Database branch reference creation/update scans commit targets, alert locations persist real commit/blob/path coordinates, and public secret scanning tests use committed secret patterns instead of an internal operator alert seed route.

Closed BUG-2480 by removing the undocumented `node_id` field from Git Database blob-create responses. `POST /api/v3/repos/{owner}/{repo}/git/blobs` now matches the OpenAPI response-shape ratchet.

Closed BUG-2481 by making Bleephub Dependabot alerts derive from public dependency graph snapshots and published security advisories. Repository security advisories now persist GitHub vulnerability package coordinates; successful default-branch dependency snapshots create matching Dependabot alerts from the global advisory database; publishing an advisory creates alerts from already submitted dependency snapshots; and the old operator-only Dependabot alert seed endpoint was removed.

Closed BUG-2482 by making the AWS simulator software development kit Amazon Elastic Container Service long-running task test poll real task state through `DescribeTasks`. `TestECS_TaskNoCommandStaysRunning` no longer assumes task startup completed after a fixed sleep, while still asserting that the no-command task reaches and remains `RUNNING` without container exit codes.

Closed BUG-2483 by making Bleephub secret scanning push protection mint bypass placeholders from protected public writes. Public contents writes and Git Database branch reference creation/update now detect enabled provider patterns before mutating git state, return a `422` push-protection response with a placeholder, honor active public bypasses for the matched token type, and no longer expose the internal operator placeholder seed route.

Closed BUG-2484 by removing the obsolete internal code scanning alert seed endpoint. Code scanning alert tests and downstream campaign/autofix coverage already created alert state through GitHub's public SARIF upload route, so `/internal/repos/{owner}/{repo}/code-scanning/alerts` no longer existed in the route table, and source guards rejected reintroducing that operator shortcut in either tests or server registration.

Closed BUG-2485 by removing the obsolete internal secret scanning alert seed endpoint. Secret scanning alert tests already created alert state from committed repository content and Git Database branch reference writes, so `/internal/repos/{owner}/{repo}/secret-scanning/alerts` no longer existed in the route table, and source guards rejected reintroducing that operator shortcut in either tests or server registration.

Validation in this branch included:

```bash
bash -n scripts/bleephub-local-dev.sh
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestPersistentServerStorageRequiresDurableGitAndObjectBytes|TestReleases_AssetBytesUseObjectStore|TestPackageAndRegistryBytesUseObjectStore|TestContainerRegistryPublishCreatesPackageVersion|TestReleases_AssetLifecycle|TestDeleteRepo' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestPackageAndRegistryBytesUseObjectStore|TestContainerRegistryPublishCreatesPackageVersion|TestPackages_' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'Test(DeleteRepoS3GitCleanupFailurePreservesRepo|GitDeleteCleanup|UnitDeleteRepo)$' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'Test(DeleteRepoPurgesRepositoryPackageObjectBytes|PackageAndRegistryBytesUseObjectStore|PersistenceReload_DeleteRepoLeavesNoResidue)' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'Test(Packages_|ContainerRegistry|PackageAndRegistryBytesUseObjectStore|DeleteRepoPurgesRepositoryPackageObjectBytes|PersistenceReload_DeleteRepoLeavesNoResidue|DeleteRepo)' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'Test(CodeQLDatabases_(RoundTrip|BytesUseObjectStore)|AgentsCodeScanPersistenceReload|PersistenceReload_(DeleteRepoLeavesNoResidue|RenameRepoMovesRepoScopedMetadata))' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'Test(CodeQL|CodeScanning|RegisteredAPIv3RoutesExistInGitHubSpec|FuzzRoutePatternsMatchRegisteredRoutes|PersistenceReload_DeleteRepoLeavesNoResidue|DeleteRepo)' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'Test(RepoAttestations_|OrgAttestations_|UserAttestations_|Attestations_CursorPagination|ArtifactMetadataAndAttestationPersistenceReload|PersistenceReload_DeleteRepoPurgesIssueAndPullChildren|PersistentServerStorageRequiresDurableGitAndObjectBytes)' -count=1
bash -n scripts/bleephub-local-dev.sh
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -count=1
bash scripts/check-latest-deps.sh
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'TestGitHub(CommandLineInterface|SoftwareDevelopmentKit)HarnessUsesAdminOrganizationAPI|TestAdminCreateOrg' -count=1
(cd bleephub/sdk-tests && GOCACHE=/private/tmp/sockerless-go-cache GOWORK=off go test -run 'Test(Organizations|AppsInstallationTokenFlow|OrgProfileTeamsAndMembershipSurfaces|OrgWebhooksSDK)$' -count=1)
(cd bleephub/sdk-tests && GOCACHE=/private/tmp/sockerless-go-cache GOWORK=off go test -count=1)
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'TestPublicFeatureTestsProvisionOrganizationsThroughAdminAPI|Test(GetOrg|UpdateOrg|DeleteOrg|ListAuthUserOrgs|CreateTeam|ListTeams|GetTeam|DeleteTeam|OrgMembership|RemoveMembership|TeamRepoPermission|ListUserTeams|GraphQLViewerOrgs|GraphQLOrganization|CreateOrgRepo|CreateOrgRepoExtended|ListOrgRepos|RepoOrganizationField|OpenAPIOrg|GetRepoInstallationHTTP|InviteFlow|PublicizeAndConcealMembership|OrgProfileTeamsAndMembershipSurfaces|OrgWebhooks|Codespaces|AppsInstallationTokenFlow|CreateRepositoryInOrganization|Actions.*Org)' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'Test(CodeScanning(AlertTestsUsePublicSARIFUpload|_ListAndFilter|_GetAndInstances|_PatchDismiss|_InvalidDismissedReason|_SARIFUploadCreatesAlerts|OrgAlerts|Autofix|AutofixEligibility)|LiveCodeScanning_FullFlow|OrgCampaigns)' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -count=1
pre-commit run ui-typecheck-bleephub --all-files
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'TestSecretScanning|TestLiveSecretScanning_CRUD' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'TestGitData|TestUpdateRef|TestSecretScanning_GitDatabaseRefCreatesAlert|TestGetBlob|TestCreateBlob|TestListRefs|TestGetRef' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'TestDependabot|TestLiveDependabot|TestEnterpriseDependabot|TestDependencyGraph|TestGlobalSecurityAdvisories|TestSecurityAdvisories' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -count=1
(cd simulators/aws/sdk-tests && GOCACHE=/private/tmp/sockerless-go-cache GOWORK=off CGO_ENABLED=0 go test -v -count=1 -timeout 180s -run TestECS_TaskNoCommandStaysRunning .)
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'TestSecretScanning|TestGitData|TestUpdateRef|TestCreateRef|TestCreateBlob|TestListRefs|TestGetRef' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'TestRegisteredAPIv3RoutesExistInGitHubSpec|TestFuzzRoutePatternsMatchRegisteredRoutes|TestSecretScanning|TestGitData|TestUpdateRef|TestCreateRef|TestCreateBlob|TestListRefs|TestGetRef' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'Test(CodeScanningAlertTestsUsePublicSARIFUpload|RegisteredAPIv3RoutesExistInGitHubSpec|FuzzRoutePatternsMatchRegisteredRoutes|CodeScanning|LiveCodeScanning|OrgCampaigns)' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'Test(SecretScanningAlertTestsUseCommittedContent|RegisteredAPIv3RoutesExistInGitHubSpec|FuzzRoutePatternsMatchRegisteredRoutes|SecretScanning|LiveSecretScanning|GitData|UpdateRef|CreateRef|CreateBlob|ListRefs|GetRef)' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -count=1
pre-commit run --all-files
```

## 2026-07-10 - Bleephub Repository Codespace Cleanup (`feat/bleephub-repository-codespace-cleanup`)

This branch continued from merged #784, which closed the broad Bleephub repository-deletion durable-state cascade for issue/pull request child state, repository-ID keyed rows, selected-repository references, deployment state, environment state, GitHub Pages deployment records, team grants, artifact metadata, source import records, dependency snapshots, SBOM exports, enterprise Dependabot repository-access IDs, labels, milestones, and reaction parent buckets.

Closed BUG-2468 by making repository deletion clean Codespace runtime state before deleting repository records. Repository deletion now removes backing Codespace containers and workspace directories through the same fail-loud path as direct Codespace deletion, and REST/GraphQL callers surface cleanup failures instead of deleting only the SQLite row.

Closed BUG-2469 by hardening the `sim (aws sdk)` CI job against GitHub-hosted runner disk exhaustion. The job now frees regenerable Go/Docker/apt caches before the large AWS SDK simulator shard, runs the prebuilt SDK test binary directly, and passes the prebuilt simulator binary into the SDK test harness instead of rebuilding the simulator during execution.

Closed BUG-2470 by making persisted Bleephub require object-backed GitHub Actions byte storage. Startup now refuses SQLite persistence unless the Actions artifact/cache/log byte store has been initialized from `BLEEPHUB_OBJECT_S3_BUCKET`, so a restarted service cannot advertise durable CI/CD records whose bytes lived only in memory or local files. The local development launcher fails loudly until object-store coordinates are supplied, and the persistence documentation now names the same requirement.

Closed BUG-2391 through BUG-2398 by wiring repository REST/GraphQL metadata to persisted repository, git, Pages, and viewer-access state. Licensed repositories exposed `Repository.licenseInfo`; discussion/issues/wiki settings and merge-method settings flowed through REST and GraphQL; Pages capability, pushed timestamps, archival timestamps, template provenance, and repository permissions stopped using constants or fabricated defaults.

Closed BUG-2399 by rebalancing the AWS Command Line Interface simulator appdata/appdata2 shards while preserving required check names.

Closed BUG-2400 and BUG-2401 by making pull request GraphQL status rollups use both REST commit statuses and check runs, and by adding GitHub's top-level `avatar_url` to REST commit status responses.

Closed BUG-2414 by making release asset upload follow GitHub's raw upload contract. Bleephub now registers the advertised `/api/uploads/repos/{owner}/{repo}/releases/{id}/assets{?name,label}` route, reads metadata from the query string, stores the raw request body with the request content type, and no longer accepts multipart/form fallback bytes.

Closed BUG-2402 through BUG-2405 and BUG-2413 by making Codespaces fail loudly. Codespace records are persisted only after workspace/container creation succeeds; image pull failures do not fall back to `ubuntu:latest`; start/stop/delete return errors on required backend failure; delete preserves state after failed container cleanup; and random-name generation requires cryptographic entropy rather than timestamp fallback.

Closed BUG-2406 by making OAuth device flow require browser approval. Device codes start pending, token polling returns `authorization_pending` until approval, and the final token belongs to the approving logged-in user.

Closed BUG-2407 by snapshotting code-quality setup records at the store boundary so failed validation cannot mutate persisted setup state through escaped slices or timestamps.

Closed BUG-2408 through BUG-2412 by removing fabricated Actions repository/ref/SHA context. Repository-scoped workflow paths resolve refs through real git storage and reject unresolved refs; repo-less internal submissions omit repository context instead of claiming `bleephub/test`; missing repository scope fails job/message construction loudly; webhook test deliveries require a real default-branch commit; and run-control tests seed real repositories before exercising repo-scoped runs.

Closed BUG-2415 by making the Bleephub runner UI use GitHub's repository-scoped Actions runners REST endpoint for its primary inventory. The page no longer fetched `/internal/sessions`, and its coverage asserted the public repository and runner routes while rejecting internal session access.

Closed BUG-2416 by replacing unexplained shorthand in Bleephub UI source comments with descriptive dashboard, user profile, and organization page names.

Closed BUG-2417 by making GitHub Pages deployment creation advertise the GitHub-compatible status URL and making status/cancel lookup resolve the public deployment/build identifier as well as the internal record ID.

Closed BUG-2418 by centralizing checked cryptographic randomness for Bleephub token, secret, invite-code, advisory, gist, and OpenID Connect token identifier generation. Ignored `crypto/rand.Read` calls were removed, timestamp fallback identifiers were removed, and a source guard now rejects unchecked entropy reads.

Closed BUG-2419 by making GitHub Actions artifact finalization and signed-download URL lookup use the workflow run backend identifier when it is supplied. Same-name artifacts from concurrent runs no longer cross-finalize or cross-download, matching the existing list scoping.

Closed BUG-2420 by scoping public GitHub Actions run, attempt, job, log, cancel, rerun, delete, artifact, concurrency, and protection endpoints to the repository named in the GitHub REST path. Global workflow run IDs and stable job IDs no longer resolve across repositories after only checking the requested repository's readability.

Closed BUG-2421 by making Bleephub notification thread identity type-safe. Issue and pull request notification threads now use distinct typed IDs, read/done/subscription state keys no longer collide across resource types, advertised notification thread URLs use `/api/v3/notifications/...`, and old numeric-only notification store helpers were removed.

Closed BUG-2422 through BUG-2425 by moving Bleephub account-management, audit-log, and OAuth UI paths off operator-only management routes. Organization and team management now uses GitHub Enterprise Server/public GitHub REST organization and team routes instead of `/internal/orgs` and `/internal/teams`. User administration now uses GitHub Enterprise Server user list/create/delete/site-admin routes instead of `/internal/users`; Bleephub also persists account suspension state and rejects suspended user tokens with `403`. The audit-log page now reads organization audit logs through `/api/v3/orgs/{org}/audit-log` using GitHub's phrase/order query model, and the server applies ascending audit-log order. The OAuth page now starts web/device flows and polls device tokens through `/login/oauth/authorize`, `/login/device/code`, and `/login/oauth/access_token` instead of rendering pending server-side codes from `/internal/oauth/state`.

Closed BUG-2426 by backing Bleephub browser sessions with real stored credentials. `/login` now requires a stored personal access token for the submitted account, rejects arbitrary password strings and mismatched tokens, refuses suspended accounts, and invalidates existing browser sessions when the account becomes suspended. OAuth web-flow consent and device-flow approval therefore run under a real authenticated Bleephub user instead of a login-name-only session.

Closed BUG-2427 by requiring real registered OAuth clients across Bleephub OAuth flows. Device-code issuance now rejects unknown `client_id` values, device-token polling requires the same client ID that issued the code, authorization-code consent requires a registered OAuth App or GitHub App client, and the token exchange validates the matching client secret before minting a user-to-server token.

Closed BUG-2428 by keeping the Bleephub OAuth UI on the same registered-client contract as the service. The OAuth flow controls no longer rely on a fake default client identifier, and the user-entered registered `client_id` is included in the web authorization URL, device-code request, and device-token polling request.

Closed BUG-2429 by fixing hook-discovered stale coverage and dead UI types. The pending-deployment review flow fixture now creates a real workflow file through the public contents API before submitting a repo-scoped workflow, the GitHub Enterprise Server-only user-administration and Pages deployment status routes are explicitly allowlisted in the route-spec guard, and obsolete runner-session TypeScript exports were removed after the runner UI moved to GitHub Actions public runner endpoints.

Closed BUG-2430 by making the local Bleephub Go pre-commit hook truthful during the temporary local Docker outage. During the outage, the local hook ran the non-Docker Bleephub suite while Docker-backed Codespaces lifecycle coverage remained fail-loud in CI instead of silently pretending the missing local Docker socket was covered.

Closed BUG-2431 by upgrading the stale AWS and Google Cloud Go modules surfaced by pre-push dependency freshness. The affected Amazon EC2 software development kit, Google API client, and Google Cloud Firestore module pins were brought to their latest published versions, and dependency freshness passed again.

Closed BUG-2432 by removing hidden admin-owned identity defaults from GitHub App seed configuration. Seeded GitHub Apps now require an explicit existing owner user, installations require an existing target user or organization with a matching target type, persisted app owners are validated on load, and app JSON no longer fabricates a Simple User when app owner state is corrupt.

Closed BUG-2433 by renaming the Bleephub runner integration harness's Google Cloud service-account credential generation from fake service-account JSON to simulator service-account JSON. The harness still generated a real RSA key and drove the Google client JWT signing and token exchange path, with only the token endpoint coordinate pointed at the simulator.

Closed BUG-2434 by restoring the local Bleephub Go pre-commit hook to the full Bleephub suite after Docker compatibility returned on the host. The temporary non-Docker skip script was removed, so Docker-backed Codespaces coverage ran locally again instead of being deferred to CI.

Closed BUG-2435 by making Docker-backed Make targets load local images correctly across Docker frontends. The shared build helper uses `docker buildx build --load` when Buildx is available and legacy `docker build` otherwise, so smoke, Bleephub runner, Bleeplab runner, and Bleephub `gh` command-line interface harness images are available to the following `docker run` step under Docker Engine and Podman compatibility.

Closed BUG-2436 by correcting the Bleephub `gh` command-line interface documentation to name the actual required `Bleephub GitHub command-line interface` CI job.

Closed BUG-2437 by making GitHub Actions workflow dispatch resolve GitHub `ref` inputs through git storage the way official clients send them. Dispatch now accepts full refs, branch names such as `main`, tag names, and raw commit SHAs, stores the resolved ref/SHA on the workflow run, and still returns a loud `422` for unresolved refs. The real `gh workflow run ci.yml --ref main` path passed in the Docker-backed command-line interface harness.

Closed BUG-2438 by removing the remaining user-facing Bleephub UI dependency on operator-only metrics/status/storage diagnostics. The overview and metrics pages now aggregate workflow runs, jobs, job conclusions, and online runners through public GitHub REST repository Actions routes; tests assert those pages do not call `/internal/metrics`, `/internal/status`, or `/internal/storage`. The storage-coordinate page was removed from the routed UI instead of wrapping non-GitHub persistence details in a user-facing product surface.

Closed BUG-2439 by deleting the dead `formatUptime` helper after process uptime stopped appearing in user-facing Bleephub pages.

Closed BUG-2440 by splitting the Bleephub production UI bundle at real route and dependency boundaries. `App.tsx` lazy-loads page modules through the router, and Vite now emits explicit vendor chunks for React, TanStack, YAML, cryptography, and miscellaneous third-party code without raising Vite's chunk warning threshold. The production build no longer emits large-chunk or circular-chunk warnings.

Closed BUG-2442 by updating Bleephub Playwright end-to-end coverage to the public GitHub Actions metrics contract. The Operations console now expects the `Workflow runs` metrics label exactly, the metrics page checks the `GitHub Actions throughput` heading, and fault-injection coverage fails `/api/v3/user/repos` instead of the removed `/internal/metrics` diagnostic route.

Closed BUG-2443 by making the AWS simulator's Amazon Simple Queue Service `ReceiveMessage` honor long polling. Empty receives now wait up to `WaitTimeSeconds`, available messages still return immediately, and invalid wait times outside the real 0-20 second range fail loudly. The AWS SDK test harness now runs the main simulator at warning level so successful request traffic cannot flood CI logs.

Closed BUG-2444 by adding the missing AWS Budgets CloudTrail event-source mapping. AWS Budgets management calls now record the real `budgets.amazonaws.com` event source instead of emitting fail-loud "no eventSource mapping" warnings, and the mapping unit coverage pins the service prefix.

Closed BUG-2445 by exposing GraphQL `Release.immutable` from the same persisted immutable-release state used by the REST endpoints. Repository release connections, release-by-tag lookup, and latest-release lookup now derive the field from repository-level toggles plus organization all/selected enforcement instead of hiding the field to make official clients fall back.

Closed BUG-2446 by making GraphQL pull request status-rollup connections expose the official GitHub command-line interface count-by-state fields from the same commit-status and check-run stores that back the node list. Actions-created check suites now persist their workflow-run identifiers, workflow name, and workflow file metadata, so `CheckRun.checkSuite.workflowRun.workflow.name` resolves from real Actions state instead of returning null.

Closed BUG-2448 by updating the GraphQL sweep test header to name GitHub command-line interface version 2.96 as the source for the replayed GraphQL shapes used by the current status-rollup coverage.

Closed BUG-2447 by persisting GitHub Actions workflow runs and archived attempts in SQLite. Run creation, dispatch state transitions, cancellation, deployment review, rerun archive/restore, startup-failure runs, repository rename/delete, and run deletion now keep the durable run records coherent; non-terminal runs reload as completed/cancelled because runner dispatch state is process-local and cannot truthfully continue after a service restart.

Closed BUG-2449 by returning fail-loud GitHub API errors for public secure-random generation paths that had still panicked. GitHub App manifest conversion, seeded GitHub App secrets, OAuth App creation, OAuth web/device token issuance, installation tokens, gist create/update/fork identifiers, security advisory and CVE identifiers, Classroom invite codes, OpenID Connect signing keys/token IDs, hosted-compute network settings/configuration IDs, GitHub Actions runner registration/removal tokens, and Actions cache download tokens now propagate entropy failures to their HTTP handlers; cache reservation avoids creating partial cache records when token generation fails.

Closed BUG-2450 by moving OAuth App token reset and scoped-token creation onto the error-returning user-to-server token path. Reset now mints the replacement before revoking the original token, so entropy or persistence failure returns a fail-loud GitHub API error without destroying the existing credential.

Closed BUG-2451 by moving the Docker-backed `gh` command-line interface parity harness's organization provisioning onto GitHub Enterprise Server's public admin organization API. The harness no longer calls `/internal/orgs`, and Go coverage now rejects that operator-only route in the official-client harness.

Closed BUG-2452 by making the Bleephub enterprise UI consume the configured enterprise slug at runtime. `/health` now reports the service's `BLEEPHUB_ENTERPRISE_SLUG`, Enterprise page copy displays that slug, and all enterprise UI REST helpers build `/api/v3/enterprises/{enterprise}/...` paths from that runtime coordinate instead of hardcoding the default `bleephub` slug.

Closed BUG-2453 by removing the Bleephub UI test setup's localStorage warning source. The setup now installs jsdom localStorage without first touching Node's warning-producing localStorage getter, so localStorage-backed auth paths still run and Vitest output stays clean.

Closed BUG-2454 by moving fine-grained personal access token generation onto an injectable full-read entropy helper. The helper now returns a normal error when secure randomness is unavailable, and entropy failure is covered directly alongside the other credential helpers.

Closed BUG-2455 by persisting Bleephub gist state in SQLite-backed service storage. Gists, comments, stars, forks, histories, comment counters, and sequence counters now reload as durable service state instead of disappearing on process restart.

Closed BUG-2456 by replacing the stale Bleephub persistence bucket inventory comment with a pointer to the actual `loadBucket` registrations. The code no longer carried a duplicate manual list that drifted when durable state buckets changed.

Closed BUG-2457 by making repository deletion purge the persisted child state attached to the deleted repository's issues and pull requests. Issue comments, issue events, sub-issue links, issue dependency links, pull request reviews, and pull request review comments no longer survived a SQLite reload or attached to a later repository that reused issue or pull request IDs.

Closed BUG-2458 by extending repository deletion to repository-ID keyed rows and selected-repository references. Artifact attestations, repository activity, clone traffic, watch subscriptions, GitHub App selected repositories, installation token repository scopes, organization Actions settings, runner groups, Actions secrets/variables, agent secrets/variables, Dependabot access and org secrets, Codespaces org secrets, Copilot coding-agent permissions, private registries, immutable-release enforcement, and code-security attachments no longer survived deletion with the old repository ID.

Closed BUG-2459 by extending repository deletion to deployments, deployment statuses, environments, environment branch policies, environment protection rules, and GitHub Pages deployment records. Those repository-ID keyed rows no longer survived SQLite reload or attached to a later repository that reused the old ID.

Closed BUG-2460 by making deployment deletion purge the deployment's status rows from memory and SQLite with the deployment record.

Closed BUG-2461 by moving team repository access lists and permission overrides during repository rename and transfer, and by removing those team grants when the repository is deleted.

Closed BUG-2462 by moving organization artifact storage/deployment metadata `github_repository` references during repository rename and transfer, and by deleting artifact metadata rows for a deleted repository.

Closed BUG-2463 by adding source imports, dependency graph snapshots, generated SBOM exports, and enterprise Dependabot repository-access IDs to the repository deletion cascade.

Closed BUG-2464 by adding Copilot coding agent tasks, issue field values, and CodeQL variant-analysis target rows to the repository deletion cascade. Deleted repositories and their issues no longer left those durable rows behind for a reloaded or recreated repository to inherit.

Closed BUG-2465 by adding Projects v2 content items to the repository deletion cascade and by making project deletion clear its in-memory content index. Deleted repository issues and pull requests no longer left project items behind after reload or ID reuse.

Closed BUG-2466 by adding notification state to repository deletion and rename cascades. Deleted issue and pull request threads no longer left read, done, or subscription state behind for later ID reuse, and repository rename/transfer moved repo-scoped notification read markers to the new full name.

BUG-2441 stayed open because the current Bleephub UI unused-export toolchain still emitted Node's `DEP0205 module.register()` warning after `knip` was upgraded from 6.15.0 to the current 6.23.0 release. The gate passed and dependency freshness showed no newer `knip` version.

Validation in this branch included focused Bleephub Go tests for repository metadata, pull request status rollups, commit statuses, release asset upload, Codespaces name/catalog behavior, OAuth device flow, code-quality setup, Actions secrets/variables, workflow dispatch/internal submission, repository webhook test delivery, and run-control fixtures. The latest combined focused command was:

```bash
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestReleases_AssetLifecycle|TestGenerateCodespaceNameRequiresRandomBytes|TestCodespacesUserMachines_RealCatalogValues' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestPRGraphQL_ViewDefaultFields|TestPersistenceReload_CheckRunsAndSuites' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestPersistenceReload_WorkflowRunsAndAttempts|TestWorkflowRunsListNewestFirst|TestActionsRuns_(Get|Delete|Cancel)|TestActionsRunJobs_List|TestRerunWorkflowJob_NewAttemptCarriesOtherJobs|TestApproveWorkflowRun_ReleasesGatedRun' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestEntropyHelpersReturnErrors|TestCryptoRandomReadsAreChecked|TestCreateGist|TestGitHubApp|TestOAuth|TestSecurityAdvisories|TestClassroom|TestActionsOIDC' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestOAuth(App|Check|Reset|Revoke|Scope)|TestEntropyHelpersReturnErrors|TestCryptoRandomReadsAreChecked' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestGitHubCommandLineInterfaceHarnessUsesAdminOrganizationAPI|TestAdminCreateOrg|TestCreateOrg|TestListAuthUserOrgs' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestExistingRoutesUnaffected|TestGHApiRoot' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestCryptoRandomReadsAreChecked|TestEntropyHelpersReturnErrors|TestOrgPATGrantRequests' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestPersistenceReload_GistsCommentsStarsAndForks|Test(CreateGist|UpdateGist|DeleteGist|StarUnstarGist|ListStarredGists|ForkGist|GistComments|ListGistsForAuthUser|ListPublicGists|GistCommitsAndRevision)' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestPersistenceReload_DeleteRepoPurgesIssueAndPullChildren|TestSubIssues_|TestIssueDependencies_BlockedBy|TestDeleteRepo' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestPersistenceReload_DeleteRepoLeavesNoResidue|TestPersistenceReload_DeleteDeploymentPurgesStatuses|TestPersistenceReload_DeploymentsStatusesEnvironments|TestDeployments_Lifecycle' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestPersistenceReload_DeleteRepoLeavesNoResidue|TestPersistenceReload_RenameRepoMovesRepoScopedMetadata|TestPersistenceReload_TransferRepoMovesRepoScopedMetadata' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestPersistenceReload_DeleteRepoLeavesNoResidue|TestPersistenceReload_DeleteRepoPurgesIssueAndPullChildren' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestPersistenceReload_DeleteRepoPurgesIssueAndPullChildren|TestProjectV2' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestPersistenceReload_DeleteRepoPurgesIssueAndPullChildren|TestPersistenceReload_RenameRepoMovesRepoScopedMetadata|TestPersistenceReload_TransferRepoMovesRepoScopedMetadata|TestNotifications_' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -count=1
```

They passed with sandbox escalation for loopback listeners.

Docker compatibility was available again through Podman 6.0.1, container listing worked, and a minimal container run passed:

```bash
docker version
docker ps
docker run --rm alpine:3 true
```

The full Bleephub Go pre-commit test command passed after Docker compatibility returned:

```bash
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./... -count=1 -timeout 300s
```

The Bleephub UI validation passed after the public Actions metrics and route-level code-splitting changes:

```bash
bun run test src/__tests__/EnterprisePage.test.tsx src/__tests__/api.test.ts src/__tests__/OverviewPage.test.tsx
bun run test src/__tests__/OverviewPage.test.tsx src/__tests__/MetricsPage.test.tsx
bun run test
bun run typecheck
bun run build
npx knip
bun outdated knip
```

`npx knip` exited successfully but still emitted the Node `DEP0205 module.register()` warning tracked as BUG-2441. `bun run build` completed without Vite large-chunk or circular-chunk warnings.

The Docker-backed Bleephub `gh` command-line interface parity harness passed with the Docker-compatible Podman runtime:

```bash
make bleephub-gh-docker-test
```

It passed again after the OAuth App token-management entropy fix, after the official-client organization provisioning fix, and after the runtime enterprise-coordinate fix, each time with 117 checks passing and 0 failing.

It also passed after gist state became durable and after repository deletion began purging persisted issue and pull request child state, with 117 checks passing and 0 failing.

The focused Bleephub Playwright coverage for the public Actions metrics UI and error paths passed after rebuilding the embedded UI binary:

```bash
bun run test:e2e -- e2e/bleephub.spec.ts --grep "Operations console|Global navigation|Metrics page"
bun run test:e2e -- e2e/errorPaths.spec.ts
```

The focused AWS simulator validation for the Amazon SQS long-polling and CloudWatch-to-Amazon SQS path passed:

```bash
GOWORK=off CGO_ENABLED=0 go test -v -count=1 -timeout 180s -run 'TestSQS_ReceiveMessageHonorsLongPollingWaitTime|TestSQS_ReceiveMessageRejectsInvalidWaitTimeSeconds|TestCloudWatch_OKActionsDispatchedToSNS' .
```

The full AWS simulator software development kit target passed with the Docker-compatible Podman runtime:

```bash
make sdk-test SDK_TEST_TIMEOUT=600s
```

The AWS simulator CloudTrail event-source mapping unit test passed:

```bash
GOCACHE=/private/tmp/sockerless-go-cache GOWORK=off CGO_ENABLED=0 go test -v -count=1 -run TestAWSEventSourceCoversAllServiceSlices .
```

The focused AWS simulator software development kit rerun for AWS Budgets, process-mode CloudWatch/SNS/SQS, process-mode Amazon Elastic Container Service managed Amazon Elastic Block Store, and Amazon SQS long polling passed:

```bash
GOWORK=off CGO_ENABLED=0 go test -v -count=1 -timeout 180s -run 'TestBudgetsCRUDSDK|TestECS_ManagedEBSRunTaskProcessMode|TestCloudWatch_AlarmSNSActionToSQS_ProcessMode|TestSQS_ReceiveMessageHonorsLongPollingWaitTime' .
```

The GraphQL release immutable-state validation passed:

```bash
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestRepoGraphQL_ReleasesConnection|TestImmutableReleases_OrgSettingsAndRepoEnforcement|TestImmutableReleases_SelectedRepositories' -count=1
```

The full Bleephub Go package test also passed after the GraphQL release schema change:

```bash
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -count=1
```

The workflow-dispatch `ref` input validation passed:

```bash
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestWorkflows_Dispatch' -count=1
```

The Docker-backed `gh` command-line interface parity harness passed:

```bash
make bleephub-gh-docker-test
```

The dependency freshness hook also passed:

```bash
bash scripts/check-latest-deps.sh
```

The GitHub App seed validation also passed:

```bash
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestSeedPreRegisteredApp|TestSeedAppIdempotentAndBadKey|TestPersistence_RoundTripAppsInstallationsTokensRepos' -count=1
```

The runner harness shell syntax check also passed:

```bash
bash -n bleephub/test/run-integration.sh
```

The full local Bleephub Go hook command also passed:

```bash
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./... -count=1 -timeout 300s
```

The runner UI validation also passed:

```bash
bun run test src/__tests__/RunnersPage.test.tsx
bun run typecheck
```

The GitHub Pages deployment validation also passed:

```bash
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestPagesDeployments_CreateStatusCancel|TestPagesHealthCheck|TestPagesBuildsCRUD|TestPersistenceReload_PagesBuildIDSequence' -count=1
```

The checked-entropy validation also passed:

```bash
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestCryptoRandomReadsAreChecked|TestGitHubApp|TestOAuth|TestPagesDeployments_CreateStatusCancel|TestSecurityAdvisories|TestClassroom' -count=1
```

The Actions artifact run-scoping validation also passed:

```bash
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestArtifact(CreateUploadFinalize|FinalizeScopesByWorkflowRunBackendID|ListReturnsFinalized|Download)|TestGetSignedArtifactURL' -count=1
```

The Actions repository-scoped run/job validation also passed:

```bash
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestActionsRunAndJobEndpointsScopeIDsToPathRepository|TestActionsRuns_(Get|Delete|Cancel)|TestActionsRunJobs_List|TestActionsJobs_(Get|Logs)|TestActionsArtifacts_ListRunArtifacts' -count=1
```

The notification thread identity validation also passed:

```bash
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestNotifications_(ListAndRead|ThreadIDsSeparateIssuesAndPullRequests|ThreadSubscription|RepoScoped|SinceAndBefore|ParticipatingFilter)|TestNotificationThreadMarkDone' -count=1
```

The user/organization/team UI route validation also passed:

```bash
bun run test src/__tests__/api.test.ts
bun run typecheck
```

The GitHub Enterprise Server user administration route changes also passed compile-only Go validation:

```bash
GOCACHE=/private/tmp/sockerless-go-cache go test -c ./bleephub -o /private/tmp/bleephub.test
```

The focused runtime Go test for the user administration routes did not execute locally because the sandbox denied loopback binds and both escalated attempts timed out in the automatic approval reviewer before execution.

The audit-log public-route validation passed:

```bash
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestAuditLogRecords' -count=1
```

The OAuth UI endpoint validation also passed:

```bash
bun run test src/__tests__/OAuthPage.test.tsx
```

The browser-authentication validation also passed:

```bash
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestOAuth_(LoginPost|Authorize|WebFlow|DeviceFlow|TokenResponse)|TestGHDeviceFlow' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -c ./bleephub -o /private/tmp/bleephub.test
```

The registered OAuth client validation used the same focused command and compile gate after the token endpoint required registered client IDs and web-flow client secrets:

```bash
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestOAuth_(LoginPost|Authorize|WebFlow|DeviceFlow|TokenResponse)|TestGHDeviceFlow' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -c ./bleephub -o /private/tmp/bleephub.test
```

The OAuth UI registered-client validation also passed:

```bash
bun run test src/__tests__/OAuthPage.test.tsx
```

The hook-discovered fixture/spec/type cleanup also passed focused validation:

```bash
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestActionsPendingDeploymentReviewFlow|TestRegisteredAPIv3RoutesExistInGitHubSpec' -count=1
npx knip
```

## Recent Merged Context

- **#782 - Persist Bleephub repository metadata and permissions from real state.** Repository license/settings/Pages/pushed/archive/template/permissions fields moved to real persisted state; AWS Command Line Interface simulator shard balance was corrected without changing required contexts.
- **#781 - Bleephub GitHub Apps, Actions, storage, and repository fidelity.** Actions artifacts/caches/logs moved to object storage; S3 filesystem tests used the AWS simulator; GitHub Apps moved to Manifest/browser installation flows; public Actions runner/workflow paths replaced internal paths; metadata persistence became SQLite-only.
- **#779 - Bleephub pull request/release fidelity.** Pull requests, reviews, releases, action downloads, CodeQL fixtures, and repository rename/transfer/delete behavior derived from real git/object storage and public GitHub-compatible paths.
- **#778 - Open issue sweep and class hardening.** Fixed the actionable open issues except upstream-blocked AzureAD and tightened mutable store snapshots across simulators.
- **#774/#773 - Bleephub UI and stress hardening.** The UI became a functional GitHub clone, docs were swept, fuzz/load/concurrency coverage found races and scale bugs, and store/indexing hot paths were hardened.
- **#770/#750/#747 - Bleephub API/UI expansion.** Large REST/UI parity waves added many GitHub surfaces and pages; old operation-count detail lives in those PRs.

## Foundation Summary

- Docker-compatible cloud backends are stateless and map Docker concepts onto cloud primitives.
- AWS, GCP, and Azure simulators are real cloud API slices with conformance/coverage ratchets and official client coverage.
- Bleephub implements GitHub Enterprise Server-shaped REST, GraphQL, Actions, GitHub Apps/OAuth, repositories, issues, pull requests, releases, packages, webhooks, checks/statuses, Pages, and UI surfaces, with more fidelity work still active.
- GitHub Actions runner and GitLab docker-executor topologies are sim-proven across container-capable backends; live-cloud validation remains open under BUG-1075.

## Shauth operator-console authentication

Sockerless-admin gained optional Shauth OpenID Connect browser authentication.
When all production coordinates were configured, the console performed
authorization-code sign-in with PKCE and nonce validation, verified the ID
token, accepted developer and administrator roles, and used short-lived signed
HTTP-only sessions. Its shared application shell displayed the signed-in name,
role, initial avatar, and logout control. Local operator use stayed unchanged
when no Shauth coordinates were configured, while partial or insecure
production configuration failed at startup. The Amazon Web Services, Google
Cloud, and Microsoft Azure simulator API endpoints were not wrapped because
their real SDK, command-line interface, and Terraform contracts remained
unchanged.

## OpenID Connect logout protocol hardening

Sockerless Admin and the shared simulator user-interface authentication module
preserved the configured issuer exactly, rejected issuer and public coordinates
containing user information, constrained discovered logout endpoints to the
configured issuer origin, and required same-origin browser evidence for logout.
Back-channel logout accepted only bounded
`application/x-www-form-urlencoded` POST bodies, rejected query tokens,
validated `iat` and the required logout event as a JSON object, and consumed
each `jti` atomically with `sid`/`sub` session revocation. Admin retained a
validated ID token only for its owning session, bounded its session by the ID
token expiry, and used the client identifier when no ID-token hint remained.
Both Admin and the simulators returned an explicit public no-cache signed-out
page after the shared Shauth session ended, so logout did not immediately enter
a new sign-in flow.

## Current-source browser validation

The shared backend Playwright harness built the current web interface and Go
binary for every run instead of reusing an untracked executable, and launched
each server through its native command-line or environment coordinate. Cloud backend
suites started the corresponding real Sockerless simulator in API-only process
mode and provisioned their prerequisite Amazon ECS cluster, Google Cloud Storage
bucket, or Azure resources through the public cloud API surface. All seven
backend interfaces validated status, navigation, resources, metrics, and their
declared favicon in 77 browser scenarios. Their HTML stopped loading Google
Fonts at runtime, leaving each production bundle self-contained. Continuous
integration gained an explicit browser matrix for Admin, every simulator, and
every backend, while pre-commit and pre-push validation covered the shared
browser shell scripts. Each Playwright web server allowed bounded cold Go
dependency compilation in continuous integration before the harness applied
its separate 30-second runtime-health deadline, while individual browser tests
retained their 30-second timeout.

## Simulator dashboard authorization boundary

The AWS, Google Cloud, and Microsoft Azure simulator dashboards registered
their `/sim/v1/*` data handlers through the same first-party OpenID Connect
authorization boundary as the rendered operator interface. Unauthenticated
browsers could no longer read dashboard inventory behind a protected shell,
while health probes and native cloud API routes retained their existing
protocol-specific contracts.

## Direct architecture release manifests

The release workflow disabled provenance attestations on each native ARM64 and
AMD64 build so the explicit architecture tags resolved directly to OCI image
manifests instead of single-platform indexes with anonymous attestation
children. The manifest job verified both architecture media types and rejected
any generic short-SHA index whose platform set differed from exactly Linux
ARM64 and AMD64. This preserved the generic multi-architecture image for Amazon
Elastic Container Service and Kubernetes while keeping the explicit tags
usable by consumers that require a single-architecture image manifest.

## Expiring back-channel logout qualification

Sockerless Admin and the shared simulator identity module required every OIDC
Back-Channel Logout token to carry an expiry later than the validation time.
The real Shauth matrix registered all four relying-party back-channel paths,
kept the public browser coordinates on their loopback origins, and rewrote only
Ory Hydra's container-to-host delivery coordinates. The browser exercised
direct and catalog entry, shared sign-on, logout from every application,
application-local signed-out return, and fail-closed re-entry. Each compiled
relying party recorded successful signed back-channel acceptance, so the
matrix could not pass solely through front-channel iframes.

## Amazon ECS attached-container task generations

Reusing a stopped attached container created a fresh Amazon ECS execution
generation. The pending record reset to Docker's created state, every start
owned a new wait channel, and a delayed poller removed only its own channel.
While the new task was pending, cloud recovery no longer selected a historical
stopped task, so attach bound to the current task's CloudWatch stream instead
of replaying the previous cycle. Default Docker networking was normalized to
bridge semantics before task tagging and cloud-state reconstruction. A real
simulator/backend integration test ran two scripts through the same attached
container ID and received each cycle's distinct output.

## Independent Sockerless Admin session credentials

Sockerless Admin required a dedicated browser-session signing secret of at
least 32 bytes whenever Shauth OpenID Connect was enabled. The confidential
OpenID Connect client secret remained limited to client authentication, so
provider credential rotation no longer invalidated locally signed state or
session values. Focused validation proved that only rotation of the dedicated
session secret invalidated existing signatures, and the complete real
PostgreSQL, patched Ory Hydra, Shauth, compiled relying-party, and Chromium
matrix passed with the separated credentials.

## Release-aware Shauth relying-party validation

Sockerless Admin and the AWS, Google Cloud, and Microsoft Azure simulator
dashboards implemented Shauth's standard `/auth/validation` contract. Each
authenticated page exposed the verified username, email, role, and immutable
application release through exact machine-readable fields and used the
application's real global logout action. Anonymous requests returned an exact
`303` to the application's own signed-out page, while arbitrary bearer material
could not authenticate a relying party. The deployed authentication
configuration required a commit or container digest so Shauth could validate
the release actually serving each public origin.

The continuous-integration harness pinned and verified a clean Shauth source
revision before starting its real PostgreSQL and patched Ory Hydra stack. It
built the current production bundles and binaries for Admin and all three
simulators, confined passwordless validator credentials to Shauth, and rejected
their presence in relying-party process environments. Eight serialized browser
jobs covered catalog and direct entry for every application, exact identity and
release fields, relying-party global logout, application-local signed-out
return, reload persistence, reauthentication, and provider logout with witness
revocation. Sockerless Admin cached validated provider discovery metadata behind
a bounded initial lookup, preventing logout requests from hanging on repeated
discovery, and validation-page content security policy allowed only the exact
Shauth origin required by the real redirect chain.

The mandatory pre-push dependency audit also advanced the Amazon Web Services
Organizations SDK test client to its current patch release. The complete
official SDK module and the repository-wide dependency freshness gate passed
with the updated module graph.

## Containerized simulator outer-host propagation

A containerized AWS simulator resolved the outer runtime's existing
`host.docker.internal` or `host.containers.internal` IPv4 coordinate before
falling back to its own default route. It propagated that exact address to
nested workloads for metadata, callbacks, and user-supplied endpoint
coordinates, so Podman's simulator and workload networks no longer confused
the simulator gateway with the actual host.

The Bleeplab runner harness added a targeted real Amazon ECS workload check
that required the exact Bleeplab health response from inside the nested task.
The same run completed the full GitLab-style pipeline, compiled and consumed
an artifact, and reached Redis through the build pod's service alias.
Sockerless's Shauth harness also isolated the standalone validator's Go module,
so the same build succeeded when Shauth was checked out beneath Sockerless's
workspace in continuous integration. The browser job selected the Go toolchain
from that pinned Shauth module, preventing the provider's compiler requirement
from drifting behind Sockerless's own toolchain.

## AWS eventing, observability, and console workflows

The AWS cloud slice connected its eventing services through real runtimes.
AWS Lambda consumed Amazon SQS event-source mappings with visibility,
acknowledgement, partial-batch failure, and dead-letter behavior, and its
asynchronous invocations honoured retry, maximum-age, and destination
configuration. Amazon EventBridge and EventBridge Scheduler delivered canonical
event envelopes to AWS Lambda, Amazon SQS, Amazon SNS, AWS Step Functions, and
Amazon CloudWatch Logs. Step Functions Task states executed supported Amazon
SQS, Amazon SNS, EventBridge, CloudWatch Metrics, and CloudWatch Logs
integrations, including callback-token completion.

Amazon SNS delivered signed HTTP and HTTPS notifications with confirmation,
filter policies, message attributes, and raw Amazon SQS delivery. Amazon
CloudWatch enforced SigV4 and AWS Identity and Access Management authorization
on its CBOR protocol, and CloudWatch Logs implemented account storage-tier and
syslog-configuration operations. CloudTrail retained its real Amazon S3-backed
trail lifecycle.

The production AWS console gained working Lambda, Step Functions, EventBridge,
EventBridge Scheduler, Amazon SNS, Amazon SQS, CloudWatch, CloudWatch Logs, and
CloudTrail resource workflows over federated public cloud APIs. The official
AWS SDK and AWS CLI suites exercised cross-service delivery, the official
HashiCorp AWS provider completed a production-shaped apply and destroy, 229
Chromium tests covered the production bundle and accessibility, and the exact
Shauth, Ory Hydra, PostgreSQL, simulator, and browser matrix created and
observed the resources through federated SigV4.

The mandatory freshness pass upgraded Microsoft Authentication Library for Go
to v1.8.0 and `docker/login-action` to v4.5.2; the complete affected Azure SDK
integration suite passed.

## 2026-07-28 - Open cloud-fidelity closure (`feat/close-all-open-fidelity-gaps`)

The sweep closed 13 defects that had been recorded at branch creation. Amazon
SQS made FIFO deduplication, group ordering, delay,
retention, visibility, maximum message size, receive counts, and dead-letter
redrive part of its real runtime. Amazon EC2 refused subnet deletion with
attached network interfaces. Amazon ECS `StartTask` executed the workload and
selected EC2, EXTERNAL, or Fargate sandbox rules from launch type. Amazon
Amplify cloned and built real repositories and deployed real ZIP/Amazon S3
artifacts instead of reporting synthetic success.

AWS Certificate Manager gained persistent signing authorities, exact X.509
import validation, real managed renewal, SMTP email validation, and its
complete 19-operation ACME control plane backed by a real RFC 8555 nonce, JWS,
external-account-binding, account, authorization, DNS challenge, CSR,
certificate, key-rollover, and revocation data plane. The AWS console managed
ACME endpoints, domains, external account binding credentials, and accounts
through the real AWS JSON operations. Amazon SNS email and email-json
subscriptions used SMTP for confirmation and signed notification delivery;
the SMS sandbox stopped manufacturing verification codes and failed loudly
without a carrier.

Google Cloud Secret Manager performed managed Cloud SQL password rotation,
Cloud KMS performed standard and trusted wrapped-key imports with real
RSA-OAEP/AES-KWP cryptography and resolved effective Autokey ancestry,
Memorystore exposed immutable ACL revisions, and Cloud Run v1/v2 projected one
shared service. Azure Files persisted signed Share ACL identifiers and exposed
them through Azure SDK, Azure CLI, and Terraform. All three simulators labelled
workloads with a run identity and launched detached abnormal-exit reapers; the
registry-trust helper operated correctly inside the Linux container harness.

The vendored-spec freshness check sampled concurrent Google Discovery
revisions and failed on definite drift, the full cloud API catalog was
refreshed, and generated simulator surface tables gained a deterministic
pre-commit/CI gate. Official AWS, Google Cloud, and Azure SDK and CLI clients,
the official Terraform providers, real Git and SMTP servers, `x/crypto/acme`,
real container workloads, and the production AWS frontend exercised the
implementations. The full AWS, Google Cloud, and Azure simulator packages,
shared execution modules, AWS and Azure Terraform suites within their host
capabilities, frontend typecheck/tests/build, cloud conformance ratchets, and
the network-backed dependency audit passed.

Publication hardening aligned the affected paths with their current external
contracts. Amazon SQS redrive flowed through normal enqueue processing, which
assigned a new message ID, millisecond enqueue timestamp, FIFO sequence, and
destination delay, while its validation audit used the current 1 MiB maximum.
Amazon ECS interpreted an omitted launch type as the cluster's EC2 capacity
instead of imposing AWS Fargate restrictions on host-network tasks. Azure
Database for PostgreSQL flexible servers persisted and returned the top-level
SKU required by the AzureRM provider, with the official Azure SDK asserting
the same response. Google Cloud Run projection validation found its resource
inside the real project collection without assuming an otherwise empty
account, and the Azure embedded-console root contract ran only in UI-bearing
builds while `noui` retained a 404. Google Cloud DNS and Artifact Registry
Discovery documents advanced to revisions 20260723 and 20260724. The affected
unit, official SDK, vendor CLI, freshness, surface-generation, and full
pre-commit gates passed.

## 2026-07-28 - Remaining cloud-fidelity closure (`feat/finish-remaining-fidelity-gaps`)

The AWS cloud slice implemented all 23 vendored AWS Private Certificate
Authority operations. It generated real RSA and elliptic-curve authority keys,
PKCS #10 certificate signing requests, signed X.509 certificate chains and
certificate revocation lists, and persisted authority lifecycle, permission,
policy, tag, and audit-report state. AWS Certificate Manager used those
authorities for private issuance, encrypted export, and revocation instead of
maintaining an independent certificate source. Official AWS SDK, AWS CLI, and
Terraform clients completed the root-authority lifecycle through the public
AWS APIs.

Amazon Data Firehose implemented its complete 12-operation vendored surface.
Direct writes, Amazon SNS subscriptions, and Amazon CloudWatch metric streams
entered one durable, concurrency-safe buffer; server-side encryption stored
records with real AES-GCM key material before delivery. Buffer size and
interval thresholds, IAM service-role authorization, AWS KMS state, all five
supported Amazon S3 compression formats, tags, destination updates, and
encryption transitions affected real runtime behavior. The official SDK,
vendor CLI, and HashiCorp AWS provider exercised the same delivery streams,
including data arriving in Amazon S3.

The production AWS console gained resource-list, create, inspect, operate, and
delete workflows for both services. Its authenticated Shauth, Ory Hydra,
PostgreSQL, simulator, and Chromium matrix created and activated a root
authority, delivered Firehose records into Amazon S3, and verified the cloud
resources through federated SigV4. The full production frontend passed 239
Chromium tests.

The sweep also removed three publication blockers discovered by external
clients. AWS Security Token Service and Microsoft Entra Workload Identity
Federation cached issuer-scoped OpenID Connect discovery and JSON Web Key Set
metadata while validating every assertion independently. The production Caddy
configuration bounded cold-upstream retries and returned an exact `503
Retry-After` response after ten seconds. Amazon Elastic Block Store snapshot,
restore, and copy paths preserved sparse extents, so the production-shaped
Terraform stack copied an 8 GiB logical block image without consuming its
logical size.

The generated API catalog and continuous-integration shard assignments covered
both new AWS services. Same-day AWS SDK dependency releases were upgraded in
every affected module. Complete simulator, official SDK, AWS CLI, Terraform,
production build, lint, dead-code, duplication, frontend, authenticated
browser, external HTTPS, specification, generated-surface, and
dependency-freshness gates passed.

The externally reviewed workload gaps were closed through the clouds' public
contracts. AWS Step Functions gained optimized and AWS SDK Amazon ECS
`RunTask` and AWS CodeBuild integrations with request/response, `.sync`,
task-token callback, failure, timeout, cancellation, and stop behavior. The
official AWS SDK launched a real Amazon ECS container and an AWS CodeBuild
container whose real AWS CLI process reached Amazon SQS through the standard
endpoint coordinate.

AWS Amplify encrypted connected-repository credentials under AWS-owned AWS Key
Management Service material and used the write-only access token for real
private Git clones. Checked-in and explicit build specifications executed in a
managed multi-language image; a private authenticated Git server and real
Python plus Node.js build produced an artifact that the hosting data plane
served. The AWS console added repository, token, platform, and build
configuration controls.

Amazon Relational Database Service gained real PostgreSQL and MySQL data
planes behind native TLS endpoints. Engine containers retained data on
volumes, master credentials stayed encrypted at rest, and IAM database
authentication tokens were verified through SigV4 and `rds-db:connect`
authorization. Stock pgx and MySQL drivers proved schema creation, insert,
select, denied tokens, and policy-authorized tokens. The AWS console added
database creation, connection guidance, IAM-token commands, and deletion.

The AWS simulator documented the standard global and per-service SDK endpoint
variables and the explicit AWS Lambda deployment/environment contract. A real
deployed Python Lambda package used bundled boto3 and standard credentials to
send to Amazon SQS. Explicit deployment remained faithful to AWS Lambda rather
than introducing simulator-side code discovery, and the repository retained
its unaudited/non-production warning because functional qualification did not
constitute an independent security audit.

The external client harnesses also stopped relying on a warm image cache.
AWS CLI and Terraform workload-image builds used Buildx `--load` when Buildx
was available, matching the AWS SDK harness, so real Lambda Runtime API images
entered the container runtime store. The affected AWS CLI cases and the
production-shaped Terraform apply, Lambda invocation, and destroy passed from
an emptied image cache. Same-day `google.golang.org/api` v0.291.0 updates
landed in all five affected Google Cloud modules; each affected suite, the
complete official Google Cloud SDK simulator suite, and the repository-wide
freshness audit passed. The final AWS console count was 239 Chromium tests,
and the authenticated Shauth matrix covered the connected Amplify and Amazon
RDS workflows.

The post-publication CI closure replaced the last CloudWatch metric-stream test
placeholders with public cloud operations. The AWS CLI created Amazon S3
buckets, IAM service roles and inline policies, and Amazon Data Firehose
delivery streams before creating, starting, stopping, tagging, and deleting
CloudWatch metric streams. The exact appdata shard passed while Firehose
existence and IAM service-role validation remained enabled.

HashiCorp AzureRM 5.0.0 landed in the Azure Container Apps and Azure Functions
modules and examples on the same branch where its release appeared. Azure
Files shares and private DNS links adopted their required resource IDs. The
production-shaped Azure simulator stack also supplied explicit Key Vault
authorization mode and migrated Event Hubs, Event Grid, Blob containers,
Tables, and File shares to the provider's new ID-based fields. All four module
and example compositions validated against the real provider, and AzureRM 5
completed a Microsoft.Subscription apply, zero-drift plan, and destroy against
the simulator.

The Google Discovery freshness gate retained the exact newest valid response
as a one-day workflow artifact whenever regional publication drift failed CI.
The hosted runner briefly observed Cloud Resource Manager v1 and v3 revision
20260715 while the maintainer and web edges still served 20260709; Google then
returned the hosted runner to 20260709 as well. The succeeding gate passed
without inventing a revision or altering the simulator contract, while a
future recurrence remained directly reproducible from its captured official
documents.

The Azure Terraform workflow stopped bootstrapping Caddy through unbounded
Cloudsmith key and repository downloads. It installed Ubuntu's signed Caddy
package through the existing retry- and timeout-bounded APT path, preserving
the job's twelve-minute budget for the real AzureRM apply, zero-drift plan,
and destroy.

AzureRM 5 then exposed its subnet wire change through that real apply:
Microsoft.Network received `properties.addressPrefixes`, while the simulator
had retained only the older singular member and tried to create host network
fabric with an empty CIDR. Subnets now preserved both public API
representations, the official Azure SDK round-tripped the plural member, and
network namespaces plus source NAT resolved their IPv4 CIDR from either
representation.

The same external apply then reached AzureRM 5's Container Apps environment
cross-field validation. A configuration that linked
`log_analytics_workspace_id` without selecting a logs destination no longer
meant Log Analytics implicitly. The production Azure Container Apps module and
the production-shaped simulator stack now set `logs_destination` to
`log-analytics`, matching the provider's public resource contract; both
configurations validated with the real provider.

Repository-wide provider validation also reached the AWS Lambda module's
Step Functions live-differential role and found four IAM policy ARNs using an
undeclared `aws_region` variable. The policy now used the module's declared
`region` input for AWS Lambda, Step Functions, and EventBridge resources, and
all six production Terraform modules validated with their real providers.
The same complete-tree pass restored canonical HCL alignment in the Amazon ECS
runner task policy.

The full frontend fan-out also exposed a timing-sensitive Microsoft Azure
portal defect. A failed resource DELETE rendered Azure Resource Manager's
error, but a concurrent Fluent dismiss event could detach the confirmation
surface around it. The shared confirmation surface now refused dismissal
while a mutation was pending, restored itself on provider failure, and reset
the prior error only on a deliberate close or new open. The complete Azure
portal package and repository frontend fan-outs passed with the error retained
in an attached, accessible dialog.

The hosted dependency gate then captured five newer official Google Discovery
documents. The repository retained those exact artifacts: Bigtable Admin v2
revision 20260725, Cloud Logging v2 revision 20260713, Pub/Sub v1 revision
20260721, and Cloud Resource Manager v1 and v3 revision 20260715. Bigtable's
new `updateMemoryLayer` method enabled and disabled a durable cluster memory
layer, enforced its etag, listed the real cluster-scoped state, and returned a
completed long-running operation. Cloud Resource Manager v3's new
`fetchResourceSemantics` method validated full resource names and returned the
resource's published semantics shape. Authenticated official-SDK transports
exercised both methods because the current generated clients had not yet
acquired their call types, and generated coverage measured all 164 Bigtable
and all 126 Cloud Resource Manager v3 methods.

AzureRM 5's successful production-shaped apply exposed two final refresh
contracts. Microsoft.OperationalInsights workspaces now returned Azure's
default `Enabled` public network access values for ingestion and query.
Microsoft.Storage File shares retained `signedIdentifiers` through their ARM
resource, projected the same stored access policies onto the Azure Files XML
data plane, and reflected data-plane updates back to ARM. The official Azure
SDK round-tripped the complete policy across create and update, while Azure
CLI round-tripped both Log Analytics defaults. These changes removed the two
updates from the provider's post-apply plan.

That zero-drift plan completed successfully and exposed one obsolete harness
assumption afterward: the test still expected Blob container and Table IDs in
their legacy data-plane URL formats. AzureRM 5's ID-based resources stored the
canonical Microsoft.Storage ARM paths returned by the APIs they used. The
external assertions now covered those ARM IDs for the Blob container, Table,
and File share.

The follow-up external workload audit replaced the remaining implementation
shortcuts behind the AWS workload surfaces. AWS CodeBuild cloned requested Git
revisions with encrypted imported credentials or AWS Secrets Manager
authentication and executed the checked-in or explicit build specification in
the project's exact configured container image. StopBuild, StopBuildBatch, and
an aborted synchronous AWS Step Functions task cancelled the underlying
container. The official AWS SDK and AWS CLI proved private clone, success,
failure, retry, batch, stop, and an AWS CLI process reaching Amazon SQS from a
Step Functions-launched Amazon ECS or CodeBuild workload.

AWS Amplify executed backend, frontend, and test pre/build/post phases in one
real managed build container. Build-spec applications, monorepo app roots and
build paths, app/branch/build-spec environment precedence, declared branch
caches, and configured artifacts governed the job. A private authenticated Git
repository ran Python and Node.js, deployed the resulting site, and restored
its cache on a second release job.

Amazon Relational Database Service used the real vendor engine for PostgreSQL,
MySQL, and MariaDB instances. ModifyDBInstance rotated the database account
while running or retained the pending secret for the next start, the volume
preserved SQL data across stop/start, and SigV4 IAM database authentication
required TLS. Stock pgx and MySQL drivers proved rejected credentials,
authorized IAM connections, password rotation, and retained rows across all
three engines.

The AWS Cloudscape console gained complete operator paths for creating,
starting, polling, stopping, and deleting CodeBuild projects and builds;
creating and operating Amplify branches and deployments; and changing RDS IAM
database authentication and master credentials. Its 241-case Chromium suite
and authenticated Shauth, Ory Hydra, PostgreSQL, simulator, and Chromium matrix
passed. The complete official AWS SDK suite, focused AWS CLI suites,
production-shaped HashiCorp AWS provider apply/destroy, and native database
driver coverage passed against the same implementations.

The pull-request dependency job then captured two newer official Google
Discovery documents from the hosted runner. Cloud Logging v2 advanced to
revision 20260724 and IAM Service Account Credentials v1 to revision 20260723.
Their public methods, paths, and schema fields were unchanged; the repository
retained the exact compressed artifacts and provenance, and the complete Google
simulator route, specification, and measured-coverage unit suite passed.

Hosted concurrency then found three ordering assumptions that local warm runs
had not exposed. AWS Amplify had rounded job start times to whole seconds, so
two releases in one second could make the hosting plane select the older
artifact by random job ID. Amplify now retained sub-second AWS timestamp
precision; the official AWS SDK private authenticated Python and Node.js build
restored and served its second-release cache in five consecutive real-container
runs.

AzureRM independently created a NAT gateway, subnet association, and public IP
prefix association. When the subnet update won that race, Microsoft Azure's
valid intermediate gateway had no public addressing yet. The simulator now
retained that control-plane association without inventing outbound behavior,
then programmed the real network fabric when the later public-prefix update
arrived. The production-shaped AzureRM apply/destroy path exercised the same
ordering.

The AWS Step Functions workload integration remained genuinely asynchronous:
a clean hosted runner needed more than one minute to provision the configured
Amazon ECS and AWS CodeBuild images. Its official AWS SDK assertion now allowed
the same multi-minute provisioning window as the cloud services and reported
the final execution status, error, and cause. The focused real-container
integration passed.

The publication freshness gate then found a new SQLite release in all four
simulator modules that consumed it. They upgraded to `modernc.org/sqlite`
v1.55.0 and the current supporting graph. The Google simulator's complete
direct-dependency refresh also upgraded Firestore to v1.24.0, Spanner to
v1.93.0, Google APIs to v0.291.0, and the latest generated APIs. Because the
root `genproto` module no longer carried the Firestore and Spanner services,
their gRPC data planes imported the canonical protobuf packages from the
official Firestore and Spanner client modules. Every affected module and the
complete official Google Cloud SDK simulator suite passed, and the authenticated
freshness audit reported no remaining drift.

The hosted specification freshness gate observed Google Cloud Run v1 and v2
Discovery revision 20260727 while the maintainer edge still served revision
20260717. Their exact compressed artifacts and provenance pins were retained.
A canonical comparison found no public method, path, or schema-field changes;
only the revisions and descriptions changed. The Google simulator route,
specification, and measured-coverage tests passed against both newer documents,
and the authenticated freshness audit accepted both pins.

The next hosted AWS SDK N–Z run showed that cold registry transfer, rather than
either cloud workload, had consumed the entire Step Functions execution
assertion window. The shard now provisioned the exact configured public Alpine
Amazon ECS image and official AWS CLI CodeBuild image before `m.Run`. Image
acquisition therefore stayed outside the per-test lifecycle deadline while the
simulator still started, observed, and cancelled the real containers. The
focused official AWS SDK integration passed.

The console accessibility checks no longer depended on whether hosted
Chromium began with browser chrome or the document as its focus origin. The
AWS, Google Cloud, and Microsoft Azure tests focused the loaded document body
before pressing Tab, then asserted the product skip link received focus first.
All three focused browser tests passed against their real console processes.

The cold-image follow-up found that the shared AWS container runtime had
rewritten explicit Amazon ECR Public image coordinates to Docker Hub. Public
registry coordinates now remained intact, so the AWS SDK shard's exact
pre-provisioned Alpine and AWS CLI images were the images Amazon ECS and AWS
CodeBuild executed. The focused official AWS SDK Step Functions integration
ran both real containers successfully.

That integration also exposed Docker's two cancellation completion paths.
When a cancelled container wait completed through the error channel, the
CodeBuild status had changed to `STOPPED` while the shell could continue and
send its delayed Amazon SQS message. Both cancellation paths now killed the
real container, and the external SDK scenario proved no post-stop side effect
escaped.

The macOS HTTPS validation recipe loaded Docker Buildx output into the local
runtime and shared the container host's PID namespace with its privileged
Linux harness. The complete production-shaped HashiCorp AWS provider graph
then applied through the real Caddy HTTPS gateway, invoked a Lambda function
inside its attached VPC, refreshed every resource, and destroyed the graph
cleanly.

The Amazon ECS backend integration had built its arithmetic workload only in
the adjacent host daemon, leaving the backend image catalog unaware of it.
The harness now loaded that real multi-stage image through the backend's Docker
Image Load API, while live-cloud runs required an explicitly provisioned
Amazon ECR coordinate. All six arithmetic cases ran in real containers and
passed their exit-code, log, label, and environment assertions.

The hosted AWS Terraform job had launched the root, Amazon ElastiCache, and
three Amazon RDS provider graphs concurrently on one runner, combining five
simulators, five HTTPS gateways, and their real database and container
workloads. Caddy had also redundantly replaced the upstream `Host` header that
formed part of each AWS Signature Version 4 canonical request. The gateway now
used Caddy's native host preservation, local package execution stayed
serialized while Terraform retained resource-level concurrency, and CI
assigned each production-shaped package to a separate hosted runner. All five
HTTPS packages completed apply, real workload or data-plane assertions, and
destroy without signature failures or cross-package resource contention.

The mandatory publication audit found `go-git` 5.19.2 after it was released
during push validation. The AWS simulator upgraded to that patch and its
current `go-billy`, expression-evaluation, and decimal transitive graph. The
complete module suite passed, and the authenticated dependency audit reported
no remaining drift.

The shared end-to-end harness had repeated the adjacent-daemon assumption for
its compiled arithmetic fixture. It now streamed the real image through every
active cloud backend's Docker Image Load API before creating the workload, so
the backend image catalog remained the source of truth. The exact hosted e2e
command and the optional path that launched a second Amazon ECS simulator and
backend both passed the compiled workload's exit-code and log assertions.

The hosted publication edge then exposed `docker/login-action` 4.6.0 after the
local audit had passed. Both immutable multi-architecture publication jobs
upgraded to the current action. Actionlint, the repository's container
publication contract, and the authenticated whole-repository freshness audit
all passed.

The native Linux AWS simulator had rewritten `host.docker.internal` in child
workload environments to the virtual machine's default route gateway, even
though Docker's `host-gateway` alias was the correct coordinate. Rewriting now
occurred only when the simulator itself ran inside a container; native
workloads retained Docker's real alias. Focused coordinate tests and the
official AWS SDK Step Functions integration passed a real Amazon ECS task,
AWS CodeBuild container, and vendor AWS CLI in 7.45 seconds.

The hosted Amazon ECS compute shard exhausted its default subnet because the
allocator only advanced a persistent cursor; addresses belonging to stopped
tasks were never made available again. Allocation was changed to derive
current ownership from live elastic network interfaces, NAT gateway addresses,
and non-stopped ECS tasks, then circularly search the usable subnet range.
Focused coverage filled a small subnet, observed real exhaustion, stopped a
task, and reused its released address.

The same investigation tightened the full task lifecycle. Startup and stop
transitions were serialized so an asynchronous launch could not publish a task
as running after it had stopped. Cleanup waited for real runtime removal in
network-dependency order—dependents, primary workload, then pause holder—and
used bounded exact-container removal when a runtime's background cleanup had
not completed. Shell-based SDK fixtures were moved to BusyBox where a shell was
the contract; the shared-localhost case used the compiled container-command
fixture with a real HTTP listener and client probe. The complete AWS simulator
module, shared runtime suite, focused multi-container case, and exact hosted
`TestE` shard passed.

That hosted rerun then cancelled the required combined simulator lint job at
the exact five-minute job ceiling while golangci-lint was still active. The
required `lint (simulators)` context and its complete module set were preserved.
Only that matrix entry received a ten-minute job and step budget; every lighter
lint shard retained five minutes. The repository's workflow-timeout gate,
timeout-parser fixtures, and actionlint all passed.

The final hosted rerun exposed two reproducibility assumptions. The root AWS
Terraform graph had not declared a HashiCorp AWS provider version while its
sibling packages still declared 6.47.0. Every graph now declared 6.50.0, and
the complete root graph passed concurrent apply, real workload assertions,
refresh, and destroy through Caddy HTTPS with runtime Smithy validation armed.

The Microsoft Azure console's failed Container Registry deletion test had
queried the confirmation dialog synchronously while React Query was settling
the error mutation. The assertion now awaited the retained accessible Fluent
dialog after Azure Resource Manager's real error rendered. All 131 Azure
console tests and the complete UI build/test fan-out passed.

All three persistent cloud simulators strengthened their SQLite durability
boundary. Database connections now use `synchronous=FULL` with WAL mode, and
the shared server shutdown path truncate-checkpoints committed WAL records,
closes the database, and reports either failure instead of returning while the
store remains open. Multi-connection regressions in the AWS, Google Cloud, and
Microsoft Azure shared modules proved the pragma on every connection, required
an empty or absent WAL after orderly close, and reopened the same data
successfully. The complete shared suites passed with real container-runtime
access.
