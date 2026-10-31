import { useRef, useState } from "react";
import {
  Copy,
  Sun,
  Moon,
  ArrowUpRight,
  LogOut,
  Pencil,
  Check,
  X,
  RotateCcw,
  Plus,
  Trash2,
  Camera,
  ImagePlus,
} from "lucide-react";
import { useLMS } from "../../context/useLMS";
import { recentContests } from "../../data/lmsData";
import {
  PageHeader,
  SectionHeader,
  Badge,
  EmptyState,
} from "../ui/Workspace";

export interface CodingPlatform {
  id: string;
  name: string;
  handle: string;
  description: string;
  url: string;
  initial: string;
}

export interface EditableContestResult {
  id: string;
  subjectCode: string;
  title: string;
  date: string;
  score: string;
  rank: string;
}

export interface AccountProfileData {
  name: string;
  initials: string;
  headline: string;
  institutionTag: string;
  cohortTag: string;
  statusBadge: string;
  sectionBadge: string;
  enrollmentId: string;
  programme: string;
  currentSemesterLabel: string;
  cohort: string;
  section: string;
  advisor: string;
  avatarUrl: string;
  bannerUrl: string;
  platforms: CodingPlatform[];
  contestResults: EditableContestResult[];
}

const STORAGE_KEY = "vedam-account-profile-v1";

const DEFAULT_PROFILE: AccountProfileData = {
  name: "Sirwagya Shekhar",
  initials: "SS",
  headline: "B.Tech Computer Science & Engineering (AI)",
  institutionTag: "Vedam School of Technology",
  cohortTag: "Cohort 2025–2029",
  statusBadge: "Enrolled · Cohort 2025–2029",
  sectionBadge: "Batch B1 · Lab group G2",
  enrollmentId: "VDM-25-CSE-042",
  programme: "B.Tech CSE (AI)",
  currentSemesterLabel: "Semester 3",
  cohort: "2025–2029",
  section: "Batch B1 · Lab group G2",
  advisor: "Dr. Nandini Krishnan",
  avatarUrl: "",
  bannerUrl: "",
  platforms: [
    {
      id: "plat-github",
      name: "GitHub",
      handle: "sirwagya",
      description: "Repositories & contributions",
      url: "https://github.com/sirwagya",
      initial: "G",
    },
    {
      id: "plat-leetcode",
      name: "LeetCode",
      handle: "sirwagya",
      description: "Problem solving & interview practice",
      url: "https://leetcode.com/u/sirwagya/",
      initial: "L",
    },
    {
      id: "plat-codechef",
      name: "CodeChef",
      handle: "sirwagya_cc",
      description: "Competitive programming",
      url: "https://www.codechef.com/users/sirwagya_cc",
      initial: "C",
    },
    {
      id: "plat-codeforces",
      name: "Codeforces",
      handle: "sirwagya",
      description: "Contests & algorithmic practice",
      url: "https://codeforces.com/profile/sirwagya",
      initial: "Cf",
    },
  ],
  contestResults: recentContests.map((c) => ({ ...c })),
};

function loadAccountProfile(): AccountProfileData {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return DEFAULT_PROFILE;
    const parsed = JSON.parse(raw) as Partial<AccountProfileData>;
    return {
      ...DEFAULT_PROFILE,
      ...parsed,
      platforms: Array.isArray(parsed.platforms)
        ? parsed.platforms
        : DEFAULT_PROFILE.platforms,
      contestResults: Array.isArray(parsed.contestResults)
        ? parsed.contestResults
        : DEFAULT_PROFILE.contestResults,
    };
  } catch {
    return DEFAULT_PROFILE;
  }
}

export function AccountScreen() {
  const {
    currentSemester,
    theme,
    toggleTheme,
    copyText,
    showToast,
    allSubjects,
    subjectByCode,
  } = useLMS();

  const [profile, setProfile] = useState<AccountProfileData>(loadAccountProfile);
  const [draft, setDraft] = useState<AccountProfileData>(profile);
  const [isEditing, setIsEditing] = useState(false);

  const avatarInputRef = useRef<HTMLInputElement | null>(null);
  const bannerInputRef = useRef<HTMLInputElement | null>(null);

  const activeData = isEditing ? draft : profile;

  const contests = activeData.contestResults.filter((c) =>
    allSubjects.some((s) => s.code === c.subjectCode),
  );

  const startEditing = () => {
    setDraft(structuredClone(profile));
    setIsEditing(true);
  };

  const cancelEditing = () => {
    setDraft(structuredClone(profile));
    setIsEditing(false);
    showToast("Changes discarded.");
  };

  const saveChanges = () => {
    const cleaned: AccountProfileData = {
      ...draft,
      name: draft.name.trim() || DEFAULT_PROFILE.name,
      initials:
        draft.initials.trim().slice(0, 3).toUpperCase() ||
        draft.name
          .trim()
          .split(/\s+/)
          .map((w) => w[0])
          .join("")
          .slice(0, 2)
          .toUpperCase() ||
        "SS",
      enrollmentId: draft.enrollmentId.trim() || DEFAULT_PROFILE.enrollmentId,
    };
    setProfile(cleaned);
    setDraft(cleaned);
    setIsEditing(false);
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(cleaned));
    } catch {
      // Ignore storage quota errors if large images were selected
    }
    showToast("Account profile saved.");
  };

  const resetDefaults = () => {
    const fresh = structuredClone(DEFAULT_PROFILE);
    setProfile(fresh);
    setDraft(fresh);
    setIsEditing(false);
    localStorage.removeItem(STORAGE_KEY);
    showToast("Account profile reset to defaults.");
  };

  const updateField = <K extends keyof AccountProfileData>(
    key: K,
    value: AccountProfileData[K],
  ) => {
    setDraft((prev) => ({ ...prev, [key]: value }));
  };

  const handleImageUpload = (
    event: React.ChangeEvent<HTMLInputElement>,
    targetKey: "avatarUrl" | "bannerUrl",
  ) => {
    const file = event.target.files?.[0];
    if (!file) return;
    if (!file.type.startsWith("image/")) {
      showToast("Please choose a valid image file.");
      return;
    }
    const reader = new FileReader();
    reader.onload = () => {
      if (typeof reader.result === "string") {
        updateField(targetKey, reader.result);
      }
    };
    reader.readAsDataURL(file);
    event.target.value = "";
  };

  const updatePlatform = (
    id: string,
    field: keyof CodingPlatform,
    value: string,
  ) => {
    setDraft((prev) => ({
      ...prev,
      platforms: prev.platforms.map((p) => {
        if (p.id !== id) return p;
        const updated = { ...p, [field]: value };
        if (field === "name" && value.trim()) {
          updated.initial = value.trim().slice(0, 2);
        }
        return updated;
      }),
    }));
  };

  const addPlatform = () => {
    const newPlatform: CodingPlatform = {
      id: `plat-${Date.now()}`,
      name: "New Platform",
      handle: "username",
      description: "Coding profile & portfolio",
      url: "https://",
      initial: "N",
    };
    setDraft((prev) => ({
      ...prev,
      platforms: [...prev.platforms, newPlatform],
    }));
  };

  const removePlatform = (id: string) => {
    setDraft((prev) => ({
      ...prev,
      platforms: prev.platforms.filter((p) => p.id !== id),
    }));
  };

  const updateContestResult = (
    id: string,
    field: keyof EditableContestResult,
    value: string,
  ) => {
    setDraft((prev) => ({
      ...prev,
      contestResults: prev.contestResults.map((c) =>
        c.id === id ? { ...c, [field]: value } : c,
      ),
    }));
  };

  const addContestResult = () => {
    const defaultSubject = allSubjects[0]?.code ?? "CS201";
    const newContest: EditableContestResult = {
      id: `ct-${Date.now()}`,
      subjectCode: defaultSubject,
      title: "New Assessment",
      date: "Oct 2026",
      score: "90 / 100",
      rank: "12 of 120",
    };
    setDraft((prev) => ({
      ...prev,
      contestResults: [newContest, ...prev.contestResults],
    }));
  };

  const removeContestResult = (id: string) => {
    setDraft((prev) => ({
      ...prev,
      contestResults: prev.contestResults.filter((c) => c.id !== id),
    }));
  };

  const handleLogout = () => {
    showToast("Signed out of student workspace.");
    window.location.hash = "/dashboard";
  };

  return (
    <section className="screen">
      <PageHeader
        title="Account"
        description="Your student profile, coding handles, recent work, and workspace preferences."
        actions={
          <div className="inline-actions">
            {isEditing ? (
              <>
                <button
                  type="button"
                  className="btn btn-quiet"
                  onClick={resetDefaults}
                >
                  <RotateCcw aria-hidden="true" />
                  <span>Reset defaults</span>
                </button>
                <button
                  type="button"
                  className="btn btn-secondary"
                  onClick={cancelEditing}
                >
                  <X aria-hidden="true" />
                  <span>Cancel</span>
                </button>
                <button
                  type="button"
                  className="btn btn-primary"
                  onClick={saveChanges}
                >
                  <Check aria-hidden="true" />
                  <span>Save changes</span>
                </button>
              </>
            ) : (
              <>
                <button
                  type="button"
                  className="btn btn-secondary"
                  onClick={startEditing}
                >
                  <Pencil aria-hidden="true" />
                  <span>Edit profile</span>
                </button>
                <button
                  type="button"
                  className="btn btn-secondary"
                  onClick={handleLogout}
                >
                  <LogOut aria-hidden="true" />
                  <span>Log out</span>
                </button>
              </>
            )}
          </div>
        }
      />

      {/* Profile Card with Banner & Overlapping Profile Picture */}
      <div className="panel profile-card">
        <div
          className="profile-banner"
          role="img"
          aria-label="Student profile cover banner"
          style={
            activeData.bannerUrl
              ? {
                  backgroundImage: `linear-gradient(180deg, rgba(15, 22, 18, 0.28) 0%, rgba(15, 22, 18, 0.55) 100%), url(${activeData.bannerUrl})`,
                  backgroundSize: "cover",
                  backgroundPosition: "center",
                }
              : undefined
          }
        >
          {isEditing ? (
            <div className="profile-banner-edit-row">
              <div className="inline-actions">
                <input
                  type="text"
                  className="account-input account-input-pill"
                  value={draft.institutionTag}
                  onChange={(e) =>
                    updateField("institutionTag", e.target.value)
                  }
                  aria-label="Institution banner tag"
                  placeholder="Institution name"
                />
                <input
                  type="text"
                  className="account-input account-input-pill"
                  value={draft.cohortTag}
                  onChange={(e) => updateField("cohortTag", e.target.value)}
                  aria-label="Cohort banner tag"
                  placeholder="Cohort tag"
                />
              </div>
              <div className="inline-actions">
                <input
                  ref={bannerInputRef}
                  type="file"
                  accept="image/*"
                  className="sr-only"
                  onChange={(e) => handleImageUpload(e, "bannerUrl")}
                />
                <button
                  type="button"
                  className="profile-banner-tag profile-banner-action"
                  onClick={() => bannerInputRef.current?.click()}
                >
                  <ImagePlus size={13} aria-hidden="true" />
                  <span>{draft.bannerUrl ? "Change cover" : "Upload cover"}</span>
                </button>
                {draft.bannerUrl && (
                  <button
                    type="button"
                    className="profile-banner-tag profile-banner-action"
                    onClick={() => updateField("bannerUrl", "")}
                  >
                    <X size={13} aria-hidden="true" />
                    <span>Remove cover</span>
                  </button>
                )}
              </div>
            </div>
          ) : (
            <>
              <span className="profile-banner-tag">
                {activeData.institutionTag}
              </span>
              <span className="profile-banner-tag">{activeData.cohortTag}</span>
            </>
          )}
        </div>

        <div className="profile-body">
          <div className="profile-top-row">
            <div className="avatar-edit-wrapper">
              <div
                className="avatar"
                role="img"
                aria-label={`Profile picture: ${activeData.name}`}
              >
                {activeData.avatarUrl ? (
                  <img
                    src={activeData.avatarUrl}
                    alt={activeData.name}
                    className="avatar-image"
                  />
                ) : (
                  activeData.initials
                )}
                {isEditing && (
                  <>
                    <input
                      ref={avatarInputRef}
                      type="file"
                      accept="image/*"
                      className="sr-only"
                      onChange={(e) => handleImageUpload(e, "avatarUrl")}
                    />
                    <button
                      type="button"
                      className="avatar-overlay-btn"
                      onClick={() => avatarInputRef.current?.click()}
                      aria-label="Upload profile photo"
                      title="Upload profile photo"
                    >
                      <Camera size={18} aria-hidden="true" />
                    </button>
                  </>
                )}
              </div>
              {isEditing && (
                <div className="avatar-meta-controls">
                  <label className="account-field-inline">
                    <span className="sr-only">Initials</span>
                    <input
                      type="text"
                      maxLength={3}
                      className="account-input account-input-initials"
                      value={draft.initials}
                      onChange={(e) =>
                        updateField("initials", e.target.value.toUpperCase())
                      }
                      placeholder="SS"
                      aria-label="Avatar initials"
                      title="Avatar initials"
                    />
                  </label>
                  {draft.avatarUrl && (
                    <button
                      type="button"
                      className="btn btn-quiet account-btn-xs"
                      onClick={() => updateField("avatarUrl", "")}
                    >
                      Remove photo
                    </button>
                  )}
                </div>
              )}
            </div>

            <div className="inline-actions">
              <button
                type="button"
                className="btn btn-secondary"
                onClick={() => copyText(activeData.enrollmentId)}
              >
                <Copy aria-hidden="true" />
                <span>Copy ID</span>
              </button>
              {isEditing ? (
                <button
                  type="button"
                  className="btn btn-primary"
                  onClick={saveChanges}
                >
                  <Check aria-hidden="true" />
                  <span>Save changes</span>
                </button>
              ) : (
                <button
                  type="button"
                  className="btn btn-secondary"
                  onClick={startEditing}
                >
                  <Pencil aria-hidden="true" />
                  <span>Edit</span>
                </button>
              )}
            </div>
          </div>

          {isEditing ? (
            <div className="account-edit-grid mt-2">
              <label className="account-field">
                <span>Full name</span>
                <input
                  type="text"
                  className="account-input"
                  value={draft.name}
                  onChange={(e) => updateField("name", e.target.value)}
                  placeholder="Full name"
                />
              </label>
              <label className="account-field">
                <span>Degree &amp; specialisation</span>
                <input
                  type="text"
                  className="account-input"
                  value={draft.headline}
                  onChange={(e) => updateField("headline", e.target.value)}
                  placeholder="Programme headline"
                />
              </label>
              <label className="account-field">
                <span>Enrollment status badge</span>
                <input
                  type="text"
                  className="account-input"
                  value={draft.statusBadge}
                  onChange={(e) => updateField("statusBadge", e.target.value)}
                  placeholder="Enrolled · Cohort 2025–2029"
                />
              </label>
              <label className="account-field">
                <span>Batch / Lab badge</span>
                <input
                  type="text"
                  className="account-input"
                  value={draft.sectionBadge}
                  onChange={(e) => updateField("sectionBadge", e.target.value)}
                  placeholder="Batch B1 · Lab group G2"
                />
              </label>
            </div>
          ) : (
            <div className="profile-summary">
              <h2>{activeData.name}</h2>
              <p>{activeData.headline}</p>
              <div className="profile-badges">
                <Badge tone="success">{activeData.statusBadge}</Badge>
                <Badge>{activeData.sectionBadge}</Badge>
                <span className="record-id">{activeData.enrollmentId}</span>
              </div>
            </div>
          )}
        </div>
      </div>

      {/* Coding Profiles */}
      <section>
        <SectionHeader
          title="Coding profiles"
          detail={
            isEditing
              ? "Edit handles, descriptions, and profile links or add another platform."
              : "Open your profile on each platform."
          }
          action={
            isEditing ? (
              <button
                type="button"
                className="btn btn-secondary"
                onClick={addPlatform}
              >
                <Plus aria-hidden="true" />
                <span>Add platform</span>
              </button>
            ) : undefined
          }
        />

        {isEditing ? (
          <div className="platform-list">
            {draft.platforms.map((p) => (
              <div key={p.id} className="panel platform-edit-card">
                <div className="platform-edit-top">
                  <input
                    type="text"
                    maxLength={2}
                    className="account-input platform-initial-input"
                    value={p.initial}
                    onChange={(e) =>
                      updatePlatform(p.id, "initial", e.target.value)
                    }
                    aria-label={`${p.name} badge initial`}
                    title="Badge initial"
                  />
                  <div className="platform-edit-title-group">
                    <input
                      type="text"
                      className="account-input"
                      value={p.name}
                      onChange={(e) =>
                        updatePlatform(p.id, "name", e.target.value)
                      }
                      placeholder="Platform name (e.g. GitHub)"
                      aria-label="Platform name"
                    />
                    <input
                      type="text"
                      className="account-input"
                      value={p.handle}
                      onChange={(e) =>
                        updatePlatform(p.id, "handle", e.target.value)
                      }
                      placeholder="Handle / username"
                      aria-label="Platform handle"
                    />
                  </div>
                  <button
                    type="button"
                    className="icon-button"
                    onClick={() => removePlatform(p.id)}
                    aria-label={`Remove ${p.name}`}
                    title={`Remove ${p.name}`}
                  >
                    <Trash2 size={16} />
                  </button>
                </div>
                <div className="platform-edit-fields">
                  <label className="account-field">
                    <span>Description</span>
                    <input
                      type="text"
                      className="account-input"
                      value={p.description}
                      onChange={(e) =>
                        updatePlatform(p.id, "description", e.target.value)
                      }
                      placeholder="What you use this profile for"
                    />
                  </label>
                  <label className="account-field">
                    <span>Profile URL</span>
                    <input
                      type="url"
                      className="account-input"
                      value={p.url}
                      onChange={(e) =>
                        updatePlatform(p.id, "url", e.target.value)
                      }
                      placeholder="https://..."
                    />
                  </label>
                </div>
              </div>
            ))}
          </div>
        ) : (
          <div className="platform-list">
            {activeData.platforms.map((p) => (
              <a
                key={p.id}
                href={p.url}
                target="_blank"
                rel="noopener noreferrer"
                className="panel platform"
                aria-label={`Open ${p.name} profile for ${p.handle} in a new tab`}
              >
                <span className="platform-initial" aria-hidden="true">
                  {p.initial}
                </span>
                <div>
                  <h3>
                    {p.name}{" "}
                    <span className="muted font-normal">/ {p.handle}</span>
                  </h3>
                  <p>{p.description}</p>
                </div>
                <ArrowUpRight aria-hidden="true" />
              </a>
            ))}
          </div>
        )}
      </section>

      {/* Recent Course Results */}
      <section>
        <SectionHeader
          title="Recent course results"
          detail={`Evaluated work from Semester ${currentSemester}.`}
          action={
            isEditing ? (
              <button
                type="button"
                className="btn btn-secondary"
                onClick={addContestResult}
              >
                <Plus aria-hidden="true" />
                <span>Add result</span>
              </button>
            ) : undefined
          }
        />
        {contests.length ? (
          <div className="panel panel-padding">
            {contests.map((c) =>
              isEditing ? (
                <div key={c.id} className="attendance-row contest-edit-row">
                  <div className="account-edit-grid contest-edit-grid">
                    <label className="account-field">
                      <span>Assessment title</span>
                      <input
                        type="text"
                        className="account-input"
                        value={c.title}
                        onChange={(e) =>
                          updateContestResult(c.id, "title", e.target.value)
                        }
                      />
                    </label>
                    <label className="account-field">
                      <span>Course</span>
                      <select
                        className="account-input"
                        value={c.subjectCode}
                        onChange={(e) =>
                          updateContestResult(
                            c.id,
                            "subjectCode",
                            e.target.value,
                          )
                        }
                      >
                        {allSubjects.map((s) => (
                          <option key={s.code} value={s.code}>
                            {s.name} ({s.code})
                          </option>
                        ))}
                      </select>
                    </label>
                    <label className="account-field">
                      <span>Score</span>
                      <input
                        type="text"
                        className="account-input"
                        value={c.score}
                        onChange={(e) =>
                          updateContestResult(c.id, "score", e.target.value)
                        }
                      />
                    </label>
                    <label className="account-field">
                      <span>Date</span>
                      <input
                        type="text"
                        className="account-input"
                        value={c.date}
                        onChange={(e) =>
                          updateContestResult(c.id, "date", e.target.value)
                        }
                      />
                    </label>
                    <label className="account-field">
                      <span>Standing / Rank</span>
                      <input
                        type="text"
                        className="account-input"
                        value={c.rank}
                        onChange={(e) =>
                          updateContestResult(c.id, "rank", e.target.value)
                        }
                      />
                    </label>
                    <div className="contest-edit-actions">
                      <button
                        type="button"
                        className="btn btn-quiet"
                        onClick={() => removeContestResult(c.id)}
                      >
                        <Trash2 size={14} aria-hidden="true" />
                        <span>Remove</span>
                      </button>
                    </div>
                  </div>
                </div>
              ) : (
                <div key={c.id} className="attendance-row">
                  <div className="attendance-row-heading">
                    <a href={`#/subjects/${c.subjectCode}`}>{c.title}</a>
                    <strong className="whitespace-nowrap">{c.score}</strong>
                  </div>
                  <p>
                    {subjectByCode(c.subjectCode)?.name} · {c.date}
                  </p>
                  <p>Rank {c.rank}</p>
                </div>
              ),
            )}
          </div>
        ) : (
          <div className="panel">
            <EmptyState
              title="No recent results"
              description="Your evaluated course contests will appear here when available."
            />
          </div>
        )}
      </section>

      {/* Student Details & Workspace Preferences */}
      <div className="two-column">
        <section className="panel panel-padding">
          <SectionHeader
            title="Student details"
            action={
              !isEditing ? (
                <button
                  type="button"
                  className="text-link"
                  onClick={startEditing}
                >
                  <Pencil size={13} aria-hidden="true" />
                  <span>Edit details</span>
                </button>
              ) : undefined
            }
          />
          {isEditing ? (
            <div className="account-details-edit-stack">
              <label className="account-field">
                <span>Enrollment ID</span>
                <input
                  type="text"
                  className="account-input"
                  value={draft.enrollmentId}
                  onChange={(e) => updateField("enrollmentId", e.target.value)}
                />
              </label>
              <label className="account-field">
                <span>Programme</span>
                <input
                  type="text"
                  className="account-input"
                  value={draft.programme}
                  onChange={(e) => updateField("programme", e.target.value)}
                />
              </label>
              <label className="account-field">
                <span>Current semester</span>
                <input
                  type="text"
                  className="account-input"
                  value={draft.currentSemesterLabel}
                  onChange={(e) =>
                    updateField("currentSemesterLabel", e.target.value)
                  }
                />
              </label>
              <label className="account-field">
                <span>Cohort</span>
                <input
                  type="text"
                  className="account-input"
                  value={draft.cohort}
                  onChange={(e) => updateField("cohort", e.target.value)}
                />
              </label>
              <label className="account-field">
                <span>Section</span>
                <input
                  type="text"
                  className="account-input"
                  value={draft.section}
                  onChange={(e) => updateField("section", e.target.value)}
                />
              </label>
              <label className="account-field">
                <span>Academic advisor</span>
                <input
                  type="text"
                  className="account-input"
                  value={draft.advisor}
                  onChange={(e) => updateField("advisor", e.target.value)}
                />
              </label>
            </div>
          ) : (
            <dl className="detail-list">
              <div>
                <dt>Enrollment ID</dt>
                <dd className="inline-actions">
                  {activeData.enrollmentId}{" "}
                  <button
                    className="icon-button"
                    aria-label="Copy enrollment ID"
                    onClick={() => copyText(activeData.enrollmentId)}
                  >
                    <Copy />
                  </button>
                </dd>
              </div>
              <div>
                <dt>Programme</dt>
                <dd>{activeData.programme}</dd>
              </div>
              <div>
                <dt>Current semester</dt>
                <dd>{activeData.currentSemesterLabel}</dd>
              </div>
              <div>
                <dt>Cohort</dt>
                <dd>{activeData.cohort}</dd>
              </div>
              <div>
                <dt>Section</dt>
                <dd>{activeData.section}</dd>
              </div>
              <div>
                <dt>Academic advisor</dt>
                <dd>{activeData.advisor}</dd>
              </div>
            </dl>
          )}
          <p className="muted text-xs mt-6">
            {isEditing
              ? "Changes are saved to your local student workspace."
              : "You can edit any field on this page using the Edit profile button."}
          </p>
        </section>

        <div className="space-y-6">
          <section className="panel panel-padding">
            <SectionHeader title="Appearance" />
            <p className="muted text-xs mb-5">
              Choose the display that feels comfortable to study in.
            </p>
            <div className="segmented" role="group" aria-label="Appearance">
              <button
                aria-pressed={theme === "light"}
                onClick={() => theme === "dark" && toggleTheme()}
              >
                <span className="inline-flex items-center gap-2">
                  <Sun size={14} />
                  Light
                </span>
              </button>
              <button
                aria-pressed={theme === "dark"}
                onClick={() => theme === "light" && toggleTheme()}
              >
                <span className="inline-flex items-center gap-2">
                  <Moon size={14} />
                  Dark
                </span>
              </button>
            </div>
          </section>

          <section className="panel panel-padding">
            <SectionHeader title="Academic view" />
            <p className="muted text-xs">
              You’re viewing Semester {currentSemester}
              {currentSemester < 3 ? " (archived)" : ""}. Use the semester
              selector at the top of any page to browse your records.
            </p>
          </section>

          <section className="panel panel-padding">
            <SectionHeader title="Session" />
            <p className="muted text-xs mb-4">
              Signed in as <strong>{activeData.name}</strong> (
              {activeData.enrollmentId}).
            </p>
            <button className="btn btn-secondary" onClick={handleLogout}>
              <LogOut aria-hidden="true" />
              <span>Log out of workspace</span>
            </button>
          </section>
        </div>
      </div>
    </section>
  );
}
