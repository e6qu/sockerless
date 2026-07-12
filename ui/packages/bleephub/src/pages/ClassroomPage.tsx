import { useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { InlineError, Spinner } from "@sockerless/ui-core/components";
import { Link, useNavigate, useParams } from "react-router";
import {
  acceptClassroomInvitation,
  createClassroom,
  createClassroomAssignment,
  fetchClassroomDashboard,
  fetchClassroomInvitation,
  exportClassroomTransition,
  importClassroomTransition,
  replaceClassroomRoster,
  updateClassroom,
  type Classroom,
  type ClassroomAutogradingTest,
} from "../api.js";
import { Blankslate, Box, Button, DialogActions, ErrorBanner, FormLabel, Modal, StateLabel } from "../components/ui.js";
import { OrganizationIcon, PeopleIcon, PlusIcon, RepoIcon } from "../components/octicons.js";

export function ClassroomPage() {
  const { classroomId, inviteCode } = useParams<{ classroomId?: string; inviteCode?: string }>();
  if (inviteCode) return <AcceptAssignment code={inviteCode} />;
  return <ClassroomManagement classroomID={classroomId ? Number(classroomId) : null} />;
}

function ClassroomHero({ onCreate }: { onCreate?: () => void }) {
  return (
    <section
      className="mb-6 flex flex-wrap items-end justify-between gap-5"
      style={{
        padding: "1.5rem",
        color: "#fff",
        borderRadius: "0.85rem",
        border: "1px solid color-mix(in srgb, var(--color-accent) 55%, var(--color-border))",
        background: "linear-gradient(125deg, #6f2cff 0%, #0969da 43%, #00a6c8 72%, #18a957 100%)",
        boxShadow: "0 12px 30px color-mix(in srgb, var(--color-accent) 24%, transparent)",
      }}
    >
      <div style={{ maxWidth: "44rem" }}>
        <div className="mb-2 inline-flex items-center gap-2" style={{ fontSize: ".78rem", fontWeight: 700, letterSpacing: ".04em", textTransform: "uppercase" }}>
          <PeopleIcon size={16} /> Bleephub Education
        </div>
        <h1 style={{ fontSize: "2rem", lineHeight: 1.12, fontWeight: 750 }}>GitHub Classroom, kept alive.</h1>
        <p className="mt-2" style={{ color: "rgba(255,255,255,.9)", maxWidth: "40rem" }}>
          Keep rosters, starter repositories, automatic assignment repositories, feedback pull requests, and GitHub Actions autograding in one familiar workflow.
        </p>
      </div>
      {onCreate && <Button variant="secondary" onClick={onCreate} style={{ background: "#fff", color: "#24292f", borderColor: "rgba(255,255,255,.7)" }}>
        <PlusIcon size={15} /> New classroom
      </Button>}
    </section>
  );
}

function ClassroomManagement({ classroomID }: { classroomID: number | null }) {
  const query = useQuery({ queryKey: ["classrooms"], queryFn: fetchClassroomDashboard });
  const [showCreate, setShowCreate] = useState(false);
  if (query.isLoading) return <Spinner />;
  if (query.isError) return <InlineError title="Failed to load classrooms" detail={String(query.error)} />;
  const classroom = classroomID ? query.data?.classrooms.find((item) => item.id === classroomID) : undefined;
  if (classroomID && !classroom) return <Blankslate title="Classroom not found">You may not administer this classroom.</Blankslate>;
  return (
    <div>
      {!classroom && <><ClassroomHero onCreate={() => setShowCreate(true)} /><TransitionPanel /></>}
      {classroom ? <ClassroomDetail classroom={classroom} /> : <ClassroomGrid classrooms={query.data?.classrooms ?? []} onCreate={() => setShowCreate(true)} />}
      {showCreate && <CreateClassroomDialog organizations={query.data?.organizations ?? []} onClose={() => setShowCreate(false)} />}
    </div>
  );
}

function TransitionPanel() {
  const client = useQueryClient();
  const [error, setError] = useState<unknown>(null);
  const exportMutation = useMutation({ mutationFn: exportClassroomTransition, onSuccess: (blob) => { const url = URL.createObjectURL(blob); const link = document.createElement("a"); link.href = url; link.download = "bleephub-classroom-transition.json"; link.click(); URL.revokeObjectURL(url); } });
  const importMutation = useMutation({ mutationFn: importClassroomTransition, onSuccess: () => client.invalidateQueries({ queryKey: ["classrooms"] }) });
  return <Box className="mb-6" style={{ borderColor: "color-mix(in srgb, var(--color-brand-purple) 45%, var(--color-border))" }}><div className="flex flex-wrap items-center justify-between gap-4" style={{ padding: "1rem", background: "linear-gradient(100deg, color-mix(in srgb, var(--color-brand-purple) 12%, var(--color-surface)), color-mix(in srgb, var(--color-brand-cyan) 10%, var(--color-surface)))" }}><div><b>Transition from GitHub Classroom</b><p className="mt-1" style={{ color: "var(--color-fg-muted)", fontSize: ".8rem" }}>Import or export the lossless JSON bundle after migrating the referenced starter and student repositories.</p></div><div className="flex flex-wrap gap-2"><Button onClick={() => exportMutation.mutate()}>Export classrooms</Button><label className="inline-flex"><input type="file" accept="application/json,.json" className="sr-only" onChange={async (event) => { try { const file = event.target.files?.[0]; if (!file) return; importMutation.mutate(JSON.parse(await file.text())); } catch (cause) { setError(cause); } }} /><span className="inline-flex cursor-pointer items-center" style={{ padding: ".34rem .85rem", border: "1px solid var(--color-border)", borderRadius: "var(--radius-md)", background: "var(--color-bg-subtle)", fontSize: ".82rem", fontWeight: 600 }}>Import transition bundle</span></label></div></div>{(error || exportMutation.error || importMutation.error) && <div style={{ padding: "0 1rem 1rem" }}><ErrorBanner>{String(error || exportMutation.error || importMutation.error)}</ErrorBanner></div>}</Box>;
}

function ClassroomGrid({ classrooms, onCreate }: { classrooms: Classroom[]; onCreate: () => void }) {
  if (classrooms.length === 0) {
    return <Blankslate icon={<PeopleIcon size={34} />} title="Create your first classroom"><span>Connect an organization, invite a roster, and publish an assignment.</span><div className="mt-4"><Button variant="primary" onClick={onCreate}>New classroom</Button></div></Blankslate>;
  }
  return (
    <div>
      <div className="mb-3 flex items-center justify-between"><h2 style={{ fontSize: "1.1rem", fontWeight: 650 }}>Your classrooms</h2><span style={{ color: "var(--color-fg-muted)", fontSize: ".82rem" }}>{classrooms.length} total</span></div>
      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
        {classrooms.map((classroom, index) => (
          <Link key={classroom.id} to={`/ui/classrooms/${classroom.id}`} style={{ color: "inherit", textDecoration: "none" }}>
            <Box style={{ height: "100%", boxShadow: "var(--shadow-sm)" }}>
              <div style={{ height: 7, background: ["#8250df", "#0969da", "#00a6c8", "#cf4a9c", "#bf8700"][index % 5] }} />
              <div style={{ padding: "1rem" }}>
                <div className="mb-3 flex items-center justify-between gap-3"><OrganizationIcon size={20} /><StateLabel state={classroom.archived ? "draft" : "open"}>{classroom.archived ? "Archived" : "Active"}</StateLabel></div>
                <h3 style={{ fontSize: "1.05rem", fontWeight: 650 }}>{classroom.name}</h3>
                <div className="mt-1" style={{ color: "var(--color-fg-muted)", fontSize: ".82rem" }}>{classroom.organization.login}</div>
                <div className="mt-4 flex gap-5" style={{ fontSize: ".8rem", color: "var(--color-fg-muted)" }}><span><b style={{ color: "var(--color-fg)" }}>{classroom.assignments.length}</b> assignments</span><span><b style={{ color: "var(--color-fg)" }}>{classroom.roster.length}</b> students</span></div>
              </div>
            </Box>
          </Link>
        ))}
      </div>
    </div>
  );
}

function ClassroomDetail({ classroom }: { classroom: Classroom }) {
  const client = useQueryClient();
  const [showAssignment, setShowAssignment] = useState(false);
  const [showRoster, setShowRoster] = useState(false);
  const archive = useMutation({ mutationFn: () => updateClassroom(classroom.id, { archived: !classroom.archived }), onSuccess: () => client.invalidateQueries({ queryKey: ["classrooms"] }) });
  return (
    <div>
      <div className="mb-5"><Link to="/ui/classrooms" style={{ color: "var(--color-accent)", fontSize: ".82rem" }}>← Classrooms</Link></div>
      <section className="mb-5 flex flex-wrap items-start justify-between gap-4" style={{ paddingBottom: "1rem", borderBottom: "1px solid var(--color-border)" }}>
        <div><div className="flex items-center gap-2"><OrganizationIcon size={24} /><h1 style={{ fontSize: "1.55rem", fontWeight: 700 }}>{classroom.name}</h1><StateLabel state={classroom.archived ? "draft" : "open"}>{classroom.archived ? "Archived" : "Active"}</StateLabel></div><p className="mt-1" style={{ color: "var(--color-fg-muted)", fontSize: ".86rem" }}>Owned by {classroom.organization.login}</p></div>
        <div className="flex gap-2"><Button onClick={() => setShowRoster(true)}><PeopleIcon size={15} /> Roster</Button><Button variant="primary" disabled={classroom.archived} onClick={() => setShowAssignment(true)}><PlusIcon size={15} /> New assignment</Button><Button variant="ghost" onClick={() => archive.mutate()}>{classroom.archived ? "Restore" : "Archive"}</Button></div>
      </section>
      <div className="mb-5 grid gap-3 sm:grid-cols-3">
        {[{ label: "Students", value: classroom.roster.length, color: "#0969da" }, { label: "Accepted repositories", value: classroom.assignments.reduce((n, a) => n + a.accepted, 0), color: "#8250df" }, { label: "Passing", value: classroom.assignments.reduce((n, a) => n + a.passing, 0), color: "#18a957" }].map((stat) => <Box key={stat.label}><div style={{ padding: "1rem", borderLeft: `5px solid ${stat.color}` }}><div style={{ fontSize: "1.5rem", fontWeight: 750 }}>{stat.value}</div><div style={{ color: "var(--color-fg-muted)", fontSize: ".8rem" }}>{stat.label}</div></div></Box>)}
      </div>
      {classroom.assignments.length === 0 ? <Blankslate icon={<RepoIcon size={34} />} title="No assignments yet">Create an individual or group assignment from a real starter repository.</Blankslate> : <div className="grid gap-3">{classroom.assignments.map((assignment) => <Box key={assignment.id}><div className="flex flex-wrap items-center justify-between gap-4" style={{ padding: "1rem" }}><div><div className="flex items-center gap-2"><RepoIcon size={17} /><b>{assignment.title}</b><span style={{ color: "var(--color-fg-muted)", fontSize: ".76rem" }}>{assignment.type}</span></div><div className="mt-2 flex flex-wrap gap-4" style={{ color: "var(--color-fg-muted)", fontSize: ".8rem" }}><span>{assignment.accepted} accepted</span><span>{assignment.submitted} submitted</span><span style={{ color: "var(--gh-open-solid)" }}>{assignment.passing} passing</span></div></div><div className="text-right"><code style={{ display: "block", color: "var(--color-accent)", fontSize: ".75rem" }}>{assignment.invite_link}</code><span style={{ fontSize: ".72rem", color: "var(--color-fg-muted)" }}>{assignment.autograding_tests?.reduce((n, test) => n + test.points, 0) ?? 0} autograding points</span></div></div></Box>)}</div>}
      {showRoster && <RosterDialog classroom={classroom} onClose={() => setShowRoster(false)} />}
      {showAssignment && <AssignmentDialog classroom={classroom} onClose={() => setShowAssignment(false)} />}
    </div>
  );
}

function CreateClassroomDialog({ organizations, onClose }: { organizations: Array<{ login: string; name: string }>; onClose: () => void }) {
  const client = useQueryClient(); const navigate = useNavigate(); const [name, setName] = useState(""); const [organization, setOrganization] = useState(organizations[0]?.login ?? "");
  const mutation = useMutation({ mutationFn: () => createClassroom({ name, organization }), onSuccess: (item) => { client.invalidateQueries({ queryKey: ["classrooms"] }); onClose(); navigate(`/ui/classrooms/${item.id}`); } });
  return <Modal title="Create a classroom" onClose={onClose}><form onSubmit={(event) => { event.preventDefault(); mutation.mutate(); }}><FormLabel id="classroom-name">Classroom name</FormLabel><input id="classroom-name" className="mb-4 w-full" value={name} onChange={(e) => setName(e.target.value)} placeholder="Introduction to Computer Science" required /><FormLabel id="classroom-org">Organization</FormLabel><select id="classroom-org" className="w-full" value={organization} onChange={(e) => setOrganization(e.target.value)} required>{organizations.map((org) => <option key={org.login} value={org.login}>{org.name || org.login} ({org.login})</option>)}</select>{mutation.error && <ErrorBanner>{String(mutation.error)}</ErrorBanner>}<DialogActions><Button type="button" onClick={onClose}>Cancel</Button><Button type="submit" variant="primary" disabled={!name.trim() || !organization || mutation.isPending}>Create classroom</Button></DialogActions></form></Modal>;
}

function RosterDialog({ classroom, onClose }: { classroom: Classroom; onClose: () => void }) {
  const client = useQueryClient(); const [value, setValue] = useState(classroom.roster.map((entry) => `${entry.login},${entry.roster_identifier}`).join("\n"));
  const mutation = useMutation({ mutationFn: () => replaceClassroomRoster(classroom.id, value.split("\n").filter(Boolean).map((line) => { const [login, ...identifier] = line.split(","); return { login: login.trim(), roster_identifier: identifier.join(",").trim() }; })), onSuccess: () => { client.invalidateQueries({ queryKey: ["classrooms"] }); onClose(); } });
  return <Modal title="Manage roster" onClose={onClose}><p className="mb-3" style={{ color: "var(--color-fg-muted)", fontSize: ".82rem" }}>One student per line: <code>github-login,roster-identifier</code>.</p><textarea className="w-full" rows={10} value={value} onChange={(e) => setValue(e.target.value)} />{mutation.error && <ErrorBanner>{String(mutation.error)}</ErrorBanner>}<DialogActions><Button onClick={onClose}>Cancel</Button><Button variant="primary" onClick={() => mutation.mutate()}>Save roster</Button></DialogActions></Modal>;
}

function AssignmentDialog({ classroom, onClose }: { classroom: Classroom; onClose: () => void }) {
  const client = useQueryClient(); const [title, setTitle] = useState(""); const [starter, setStarter] = useState(""); const [type, setType] = useState<"individual" | "group">("individual"); const [deadline, setDeadline] = useState(""); const [tests, setTests] = useState<ClassroomAutogradingTest[]>([{ name: "Tests", command: "go test ./...", points: 10 }]);
  const mutation = useMutation({ mutationFn: () => createClassroomAssignment(classroom.id, { title, type, starter_code_repository: starter, public_repo: false, students_are_repo_admins: false, feedback_pull_requests_enabled: true, deadline: deadline ? new Date(deadline).toISOString() : undefined, autograding_tests: tests }), onSuccess: () => { client.invalidateQueries({ queryKey: ["classrooms"] }); onClose(); } });
  const submit = (event: FormEvent) => { event.preventDefault(); mutation.mutate(); };
  return <Modal title="Create an assignment" onClose={onClose}><form onSubmit={submit}><FormLabel id="assignment-title">Assignment title</FormLabel><input id="assignment-title" className="mb-3 w-full" value={title} onChange={(e) => setTitle(e.target.value)} required /><FormLabel id="assignment-starter">Starter repository</FormLabel><input id="assignment-starter" className="mb-3 w-full" value={starter} onChange={(e) => setStarter(e.target.value)} placeholder="organization/starter-code" required /><div className="mb-3 grid grid-cols-2 gap-3"><div><FormLabel id="assignment-type">Assignment type</FormLabel><select id="assignment-type" className="w-full" value={type} onChange={(e) => setType(e.target.value as "individual" | "group")}><option value="individual">Individual</option><option value="group">Group</option></select></div><div><FormLabel id="assignment-deadline">Deadline</FormLabel><input id="assignment-deadline" className="w-full" type="datetime-local" value={deadline} onChange={(e) => setDeadline(e.target.value)} /></div></div><div className="mb-2 flex items-center justify-between"><b style={{ fontSize: ".85rem" }}>Autograding tests</b><Button type="button" size="sm" variant="ghost" onClick={() => setTests([...tests, { name: "", command: "", points: 10 }])}><PlusIcon size={13} /> Add test</Button></div>{tests.map((test, index) => <Box key={index} className="mb-2"><div className="grid gap-2" style={{ padding: ".75rem" }}><input aria-label={`Test ${index + 1} name`} value={test.name} onChange={(e) => setTests(tests.map((item, i) => i === index ? { ...item, name: e.target.value } : item))} placeholder="Test name" required /><input aria-label={`Test ${index + 1} command`} value={test.command} onChange={(e) => setTests(tests.map((item, i) => i === index ? { ...item, command: e.target.value } : item))} placeholder="Command" required /><input aria-label={`Test ${index + 1} points`} type="number" min={1} value={test.points} onChange={(e) => setTests(tests.map((item, i) => i === index ? { ...item, points: Number(e.target.value) } : item))} required /></div></Box>)}{mutation.error && <ErrorBanner>{String(mutation.error)}</ErrorBanner>}<DialogActions><Button type="button" onClick={onClose}>Cancel</Button><Button type="submit" variant="primary" disabled={mutation.isPending}>Create assignment</Button></DialogActions></form></Modal>;
}

function AcceptAssignment({ code }: { code: string }) {
  const navigate = useNavigate(); const query = useQuery({ queryKey: ["classroom-invite", code], queryFn: () => fetchClassroomInvitation(code) }); const [group, setGroup] = useState(""); const [rosterIdentifier, setRosterIdentifier] = useState("");
  const mutation = useMutation({ mutationFn: () => acceptClassroomInvitation(code, query.data?.type === "group" ? group : undefined, rosterIdentifier), onSuccess: (result) => navigate(`/ui/repos/${result.repository.full_name}`) });
  if (query.isLoading) return <Spinner />; if (query.isError) return <InlineError title="Assignment invitation unavailable" detail={String(query.error)} />;
  const assignment = query.data!;
  return <div style={{ maxWidth: 680, margin: "2rem auto" }}><ClassroomHero /><Box><div style={{ padding: "1.4rem" }}><div className="mb-2 flex items-center gap-2"><RepoIcon size={22} /><h1 style={{ fontSize: "1.35rem", fontWeight: 700 }}>{assignment.title}</h1></div><p style={{ color: "var(--color-fg-muted)" }}>Accepting creates your real assignment repository from <b>{assignment.starter_code_repository?.full_name}</b>, grants your access, configures feedback, and installs the autograding workflow.</p>{assignment.roster_identifier_required && <div className="mt-4"><FormLabel id="roster-identifier">Your roster identifier</FormLabel><input id="roster-identifier" className="w-full" value={rosterIdentifier} onChange={(e) => setRosterIdentifier(e.target.value)} placeholder="Student ID or email from your course roster" required /></div>}{assignment.type === "group" && <div className="mt-4"><FormLabel id="group-name">Team name</FormLabel><input id="group-name" className="w-full" value={group} onChange={(e) => setGroup(e.target.value)} required /></div>}{mutation.error && <ErrorBanner>{String(mutation.error)}</ErrorBanner>}<div className="mt-5"><Button variant="primary" onClick={() => mutation.mutate()} disabled={mutation.isPending || (assignment.type === "group" && !group.trim()) || (assignment.roster_identifier_required && !rosterIdentifier.trim())}>Accept this assignment</Button></div></div></Box></div>;
}
