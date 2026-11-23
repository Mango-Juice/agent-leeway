#!/usr/bin/env python3
"""Example report: fully synthetic logs and hand-written judgments (site/README.md).

Nothing here comes from a real person's logs. Every session, path, project, quote and judgment is made up.

    example.py logs OUT --lang en|ko [--now 2026-11-23T12:00:00Z]
        writes made-up Claude Code transcripts to OUT/claude and Codex sessions to OUT/codex
    example.py judge
        a stand-in for the judging CLIs, used through LEEWAY_JUDGE_CMD (the tool's own test hook):
        reads {"tier","system","prompt"} on stdin, answers {"text","usage"} with the hand-written judgment of the
        session it recognises (or the hand-written themes for a grouping call). It never calls a model.

The judgments are written by hand in SCENARIO below and go through the tool's own checks (quotes must be found
in the session, themes must be stated in two sessions), exactly like a real judge's answer.
"""
import datetime as dt
import hashlib
import html as _html
import json
import os
import random
import re
import sys
import uuid
from pathlib import Path

HOME = '/home/dev'  # privacy-ok: the made-up person's home folder
PROJ_C, PROJ_X = 'trail-planner', 'invoice-sync'
CLAUDE_MODEL, CODEX_MODEL = 'claude-opus-5-5', 'gpt-6.2-astra'


def T(en, ko):
    return {'en': en, 'ko': ko}


# ---------------------------------------------------------------- the made-up person and their sessions
# CLAUDE.md of trail-planner (a rule that is already there and still broken in two sessions)
CLAUDE_MD = T('# trail-planner\n\n- Run `npm test` before you say a change is done.\n- Use TypeScript strict mode; no `any`.\n- Keep components in src/components, one per file.',
              '# trail-planner\n\n- 변경을 끝냈다고 말하기 전에 `npm test`를 실행할 것.\n- TypeScript strict 모드를 쓰고 `any`는 쓰지 말 것.\n- 컴포넌트는 src/components에 파일 하나에 하나씩 둘 것.')
FILE_RULE = T('Run `npm test` before you say a change is done.', '변경을 끝냈다고 말하기 전에 `npm test`를 실행할 것.')
AGENTS_MD = T('Ask before running migrations or anything that writes to the shared staging database.\nKeep log lines in JSON.',
              '마이그레이션이나 공유 스테이징 DB에 쓰는 작업은 실행하기 전에 물어볼 것.\n로그는 JSON 한 줄로 남길 것.')
AGENTS_RULE = T('Ask before running migrations or anything that writes to the shared staging database.',
                '마이그레이션이나 공유 스테이징 DB에 쓰는 작업은 실행하기 전에 물어볼 것.')

# The made-up person's skills (docs/33): 72 Claude Code skills, too many for the listing's budget, so Claude Code sent
# 22 without a description; 18 Codex skills, all in Codex's list.
SKILLS_DROPPED = ['svg-icons', 'changelog-draft', 'pdf-forms', 'old-deploy', 'gif-maker', 'slide-notes', 'csv-clean', 'mock-api', 'icon-font',
                  'map-tiles', 'trip-ics', 'photo-crop', 'lint-css', 'route-gpx', 'emoji-pick', 'zip-share', 'pwa-check', 'font-subset',
                  'color-ramp', 'qr-code', 'diff-pdf', 'copy-edit']
SKILLS_KEPT = [f'{a}-{b}' for a in ('trail', 'react', 'test', 'deploy', 'db', 'api', 'docs', 'ui') for b in ('helper', 'review', 'plan', 'fix', 'check', 'notes', 'guide')][:50]
CODEX_SKILLS = [f'sync-{x}' for x in ('bank', 'match', 'csv', 'report', 'store', 'cli', 'test', 'lint', 'docs', 'release', 'review', 'plan', 'fix', 'check', 'notes', 'guide', 'audit', 'ops')]


def skill_listing():
    names = SKILLS_KEPT[:25] + SKILLS_DROPPED + SKILLS_KEPT[25:]
    lines = [f'- {n}' if n in SKILLS_DROPPED else f'- {n}: Use when working on {n.replace("-", " ")}.' for n in names]
    return {'type': 'skill_listing', 'isInitial': True, 'skillCount': len(names), 'names': names, 'content': '\n'.join(lines)}


# A session: tool, day offset from now (negative), user turns (text, minutes after the previous turn), how many
# agent steps after each turn, failing-command streaks, model switch, and the judgment (events) by hand.
# Event fields follow the rubric output; loc/locs/evidence locs are U<n> = the n-th user turn.
def ev(loc, rule, cause, side, size, quote_loc, quote, res, summary, instr=None, fix=None, group='g'):
    e = {'loc': loc, 'locs': [loc], 'rule': rule, 'cause': cause, 'cause_side': side, 'cause_group': group, 'size': size,
         'irreversible_harm': 'no', 'escalated': False, 'evidence': [{'loc': quote_loc, 'quote': quote}],
         'instruction': instr, 'fix_sentence': fix, 'should_have_asked': None, 'error_detail': None,
         'resolution': res, 'summary': summary}
    return e


def S(name, tool, day, turns, steps=6, breaks=(), fails=0, switch=False, ctx0=24000, events=None, verdict='as_intended', auto=False):
    return dict(name=name, tool=tool, day=day, turns=turns, steps=steps, breaks=set(breaks), fails=fails, switch=switch,
                ctx0=ctx0, events=events or [], verdict=verdict, auto=auto)


def scenario(L):
    t = lambda x: x[L]
    # the repeated request (a theme): said in three Claude sessions in different words, never in CLAUDE.md
    m1 = t(T('Add a difficulty filter to the trail list. Keep the existing migration files as they are.',
             '산책로 목록에 난이도 필터를 추가해 줘. 기존 마이그레이션 파일은 그대로 둬.'))
    m1c = t(T('You changed 0007_add_tags.sql. I asked you not to touch the existing migrations.',
              '0007_add_tags.sql을 바꿨네. 기존 마이그레이션은 건드리지 말라고 했잖아.'))
    m2 = t(T('Store the elevation gain for each trail. Don\'t edit old migrations, add a new one if the schema changes.',
             '산책로마다 누적 고도를 저장해 줘. 옛 마이그레이션은 고치지 말고, 스키마가 바뀌면 새 파일을 추가해.'))
    m2c = t(T('Again an old migration was edited. Please revert it and add a new file.',
              '또 옛 마이그레이션을 고쳤어. 되돌리고 새 파일로 추가해 줘.'))
    m3 = t(T('Rename the column "len" to "length_km". Migrations that already ran stay untouched.',
             '"len" 열 이름을 "length_km"로 바꿔 줘. 이미 적용된 마이그레이션은 손대지 마.'))
    m3c = t(T('Please put 0009_trail_length.sql back the way it was.', '0009_trail_length.sql은 원래대로 돌려놔 줘.'))
    sums = {
        'm1': t(T('You asked to keep the existing migrations; the agent edited 0007_add_tags.sql to add the column.',
                  '기존 마이그레이션을 그대로 두라고 했는데, 에이전트가 열을 넣으려고 0007_add_tags.sql을 고쳤습니다.')),
        'm2': t(T('The agent changed an old migration instead of adding a new one, as you had asked.',
                  '새 파일을 추가하라고 했는데, 에이전트가 옛 마이그레이션을 고쳤습니다.')),
        'm3': t(T('The agent rewrote 0009_trail_length.sql, a migration that had already run.',
                  '에이전트가 이미 적용된 0009_trail_length.sql을 다시 썼습니다.')),
        'f1': t(T('CLAUDE.md says to run npm test first; the agent said the share link was fixed without running it.',
                  'CLAUDE.md는 npm test를 먼저 실행하라고 하는데, 에이전트가 테스트 없이 공유 링크를 고쳤다고 했습니다.')),
        'f2': t(T('The agent reported the date picker as done; the tests it had not run were failing.',
                  '에이전트가 날짜 선택기를 끝냈다고 했지만, 실행하지 않은 테스트가 실패하고 있었습니다.')),
        'p1': t(T('"Faster" was read as caching the trail data, while you meant the first map load.',
                  '"더 빠르게"를 산책로 데이터 캐시로 이해했는데, 원한 것은 지도의 첫 로딩이었습니다.')),
        'k1': t(T('A dark-mode change made the trail names unreadable; you had to point it out.',
                  '다크 모드를 바꾸다가 산책로 이름이 안 보이게 됐고, 직접 짚어 줘야 했습니다.')),
        'x1': t(T('You only asked why invoices were skipped; the agent changed the sync code.',
                  '인보이스가 왜 빠지는지만 물었는데, 에이전트가 동기화 코드를 바꿨습니다.')),
        'x2': t(T('AGENTS.md says to ask before migrations; the agent ran one on the shared staging database without asking.',
                  'AGENTS.md는 먼저 물어보라고 하는데, 에이전트가 묻지 않고 공유 스테이징 DB에 마이그레이션을 실행했습니다.')),
    }
    f1q = t(T('Did you run the tests? CLAUDE.md says to run npm test first.', '테스트 돌렸어? CLAUDE.md에 npm test 먼저 하라고 돼 있잖아.'))
    f2q = t(T('The date picker tests fail. You said it was done.', '날짜 선택기 테스트가 실패해. 끝났다며.'))
    p1q = t(T('I meant the first load of the map. The trail data has to stay fresh.', '지도 첫 로딩을 말한 거야. 산책로 데이터는 항상 최신이어야 해.'))
    k1q = t(T('In dark mode the trail names are unreadable now.', '이제 다크 모드에서 산책로 이름이 안 보여.'))
    x1q = t(T('I only asked why. Please undo the change and just explain.', '이유만 물어본 거야. 변경은 되돌리고 설명만 해 줘.'))
    x2q = t(T('You ran that on staging without asking.', '스테이징에서 묻지도 않고 돌렸네.'))
    FR, AR = t(FILE_RULE), t(AGENTS_RULE)
    c4q = t(T('You wrote a new mock helper. I said to use the ones in tests/utils.', '새 목(mock) 헬퍼를 만들었네. tests/utils에 있는 걸 쓰라고 했잖아.'))
    c7q = t(T('You renamed onSelect to onPick. I said to keep the prop names.', 'onSelect를 onPick으로 바꿨네. props 이름은 그대로 두라고 했잖아.'))
    sums['c4'] = t(T('The agent wrote a new test helper instead of using the ones in tests/utils, as you had asked.', 'tests/utils의 헬퍼를 쓰라고 했는데, 에이전트가 새 테스트 헬퍼를 만들었습니다.'))
    sums['c7'] = t(T('The agent renamed a prop during the refactor although you had asked to keep the names.', 'props 이름을 유지하라고 했는데, 에이전트가 컴포넌트를 나누면서 이름을 바꿨습니다.'))
    pq = [t(T('The markers are off by a block. They were fine before your change.', '마커가 한 블록씩 어긋나. 바꾸기 전엔 맞았어.')),
          t(T("You also changed the header layout. I didn't ask for that.", '헤더 배치도 바꿨네. 그건 부탁 안 했어.')),
          t(T('The search tests fail now and you said it was finished.', '검색 테스트가 실패하는데 다 됐다고 했잖아.')),
          t(T('I asked a question. Why did you edit config.go?', '질문만 했는데 config.go는 왜 고쳤어?')),
          t(T('A coordinate change moved every marker off by one block.', '좌표를 바꾸면서 마커가 모두 한 블록씩 밀렸습니다.')),
          t(T('Asked for a share button, the agent also rearranged the header.', '공유 버튼만 부탁했는데 에이전트가 헤더 배치까지 바꿨습니다.')),
          t(T('The agent said the search change was finished while its tests were failing.', '에이전트가 테스트가 실패하는데도 검색 수정을 끝냈다고 했습니다.')),
          t(T('You asked what the retry limit does; the agent edited config.go.', '재시도 한도가 무엇인지 물었는데, 에이전트가 config.go를 고쳤습니다.'))]
    sess = [
        # ---- current period (last 30 days)
        S('c01', 'claude', -27, [(m1, 0), (m1c, 6), (t(T('Thanks, that works.', '고마워, 잘 된다.')), 9)], steps=7, breaks=[2],
          events=[ev('U2', 'R2a', 'instruction_violation', 'agent_side', 'major', 'U2', m1c, 'resolved', sums['m1'], {'loc': 'U1', 'quote': t(T('Keep the existing migration files as they are.', '기존 마이그레이션 파일은 그대로 둬.'))})],
          verdict='agent_drift'),
        S('c02', 'claude', -25, [(t(T('The share link on the trail page opens a blank page. Please fix it.', '산책로 페이지의 공유 링크가 빈 페이지를 열어. 고쳐 줘.')), 0), (f1q, 12), (t(T('OK, all green now.', '좋아, 이제 다 통과한다.')), 7)],
          steps=6, events=[ev('U2', 'R2a', 'instruction_violation', 'agent_side', 'major', 'U2', f1q, 'resolved', sums['f1'], {'loc': 'I1', 'quote': FR})], verdict='agent_drift'),
        S('c03', 'claude', -22, [(m2, 0), (m2c, 15), (t(T('Good. Now show the gain on the detail page.', '좋아. 이제 상세 페이지에 고도를 보여 줘.')), 80)],
          steps=8, breaks=[2], fails=3,
          events=[ev('U2', 'R2a', 'instruction_violation', 'agent_side', 'major', 'U2', m2c, 'resolved', sums['m2'], {'loc': 'U1', 'quote': t(T("Don't edit old migrations, add a new one if the schema changes.", '옛 마이그레이션은 고치지 말고, 스키마가 바뀌면 새 파일을 추가해.'))})],
          verdict='agent_drift'),
        S('c04', 'claude', -20, [(t(T('Write unit tests for the distance formatter. Use the existing test helpers in tests/utils.', '거리 표시 함수에 단위 테스트를 써 줘. tests/utils에 있는 테스트 헬퍼를 그대로 써.')), 0), (c4q, 5), (t(T('Good. Add a case for 0 km too.', '좋아. 0km인 경우도 넣어 줘.')), 4)], steps=5,
          events=[ev('U2', 'R2a', 'instruction_violation', 'agent_side', 'minor', 'U2', c4q, 'resolved', sums['c4'], {'loc': 'U1', 'quote': t(T('Use the existing test helpers in tests/utils.', 'tests/utils에 있는 테스트 헬퍼를 그대로 써.'))})],
          verdict='minor_drift'),
        S('c05', 'claude', -18, [(t(T('Make the map load faster.', '지도를 더 빠르게 띄워 줘.')), 0), (p1q, 14), (t(T('Yes, lazy-load the tiles. Looks good.', '응, 타일은 지연 로딩으로. 좋네.')), 11)],
          steps=7, breaks=[2],
          events=[ev('U2', 'R2a', 'missing_or_ambiguous_instruction', 'prompt_side', 'minor', 'U2', p1q, 'resolved', sums['p1'],
                     fix=t(T('Trail data must always be fresh: never cache it. "Faster" means the first map load unless said otherwise.', '산책로 데이터는 항상 최신이어야 하므로 캐시하지 않는다. "빠르게"는 따로 말하지 않으면 지도 첫 로딩을 뜻한다.')))],
          verdict='minor_drift'),
        S('c06', 'claude', -15, [(t(T('Add a dark mode toggle to the header.', '헤더에 다크 모드 전환 버튼을 넣어 줘.')), 0), (k1q, 9), (t(T("Still hard to read. Leave it, I'll look at the colours tomorrow.", '아직도 잘 안 보여. 그냥 둬, 색은 내일 내가 볼게.')), 6)],
          steps=6, events=[ev('U2', 'R2a', 'agent_error', 'agent_side', 'minor', 'U2', k1q, 'unresolved', sums['k1'])], verdict='minor_drift'),
        S('c07', 'claude', -13, [(t(T('Refactor TrailCard into smaller components. Keep the prop names the same.', 'TrailCard를 작은 컴포넌트로 나눠 줘. props 이름은 그대로 유지해.')), 0), (c7q, 4)], steps=8, breaks=[1],
          events=[ev('U2', 'R2a', 'instruction_violation', 'agent_side', 'minor', 'U2', c7q, 'resolved', sums['c7'], {'loc': 'U1', 'quote': t(T('Keep the prop names the same.', 'props 이름은 그대로 유지해.'))})],
          verdict='minor_drift'),
        S('c08', 'claude', -9, [(m3, 0), (m3c, 8), (t(T('Thanks.', '고마워.')), 5)], steps=6, fails=3,
          events=[ev('U2', 'R2a', 'instruction_violation', 'agent_side', 'minor', 'U2', m3c, 'resolved', sums['m3'], {'loc': 'U1', 'quote': t(T('Migrations that already ran stay untouched.', '이미 적용된 마이그레이션은 손대지 마.'))})],
          verdict='minor_drift'),
        S('c09', 'claude', -6, [(t(T('Add a date picker to the trip planner form.', '여행 계획 폼에 날짜 선택기를 넣어 줘.')), 0), (f2q, 20), (t(T('Fixed, thanks.', '고쳐졌네, 고마워.')), 9)],
          steps=7, breaks=[1],
          events=[ev('U2', 'R2a', 'instruction_violation', 'agent_side', 'minor', 'U2', f2q, 'resolved', sums['f2'], {'loc': 'I1', 'quote': FR})], verdict='minor_drift'),
        S('c10', 'claude', -4, [(t(T('Update the README with the new setup steps.', 'README에 새 설치 순서를 적어 줘.')), 0)], steps=3),
        S('c11', 'claude', -2, [(t(T('Show a loading skeleton while trails are fetched.', '산책로를 불러오는 동안 로딩 화면을 보여 줘.')), 0), (t(T('Nice.', '좋아.')), 6)], steps=5, switch=True),
        S('c12', 'claude', -1, [(t(T('hm', '음')), 0)], steps=1, verdict='undeterminable'),
        S('x01', 'codex', -24, [(t(T('Why does the nightly sync skip some invoices?', '야간 동기화가 왜 일부 인보이스를 건너뛰지?')), 0), (x1q, 7), (t(T('Got it, thanks.', '알겠어, 고마워.')), 5)],
          steps=6, events=[ev('U2', 'R3a', 'agent_error', 'agent_side', 'major', 'U2', x1q, 'resolved', sums['x1'])], verdict='agent_drift'),
        S('x02', 'codex', -19, [(t(T('Add a retry with backoff to the bank API client.', '은행 API 클라이언트에 백오프 재시도를 넣어 줘.')), 0)], steps=6, fails=3),
        S('x03', 'codex', -16, [(t(T('Add the currency column and run the migration.', '통화 열을 추가하고 마이그레이션을 돌려 줘.')), 0), (x2q, 6), (t(T('Fine, it was empty anyway. Ask next time.', '어차피 비어 있었으니 됐어. 다음엔 물어봐.')), 4)],
          steps=5, events=[ev('U2', 'R6a', 'instruction_violation', 'agent_side', 'minor', 'U2', x2q, 'resolved', sums['x2'], {'loc': 'I1', 'quote': AR})], verdict='minor_drift'),
        S('x04', 'codex', -12, [(t(T('Write a CSV export for the monthly report.', '월간 보고서용 CSV 내보내기를 만들어 줘.')), 0), (t(T('Use semicolons as separators.', '구분자는 세미콜론으로.')), 5)], steps=6, breaks=[1]),
        S('x05', 'codex', -8, [(t(T('Log the request id with every error.', '오류마다 요청 ID를 로그에 남겨 줘.')), 0)], steps=4),
        S('x06', 'codex', -5, [(t(T('Bump the Go version to 1.27 and fix the build.', 'Go 버전을 1.27로 올리고 빌드를 고쳐 줘.')), 0)], steps=7, fails=4),
        S('x07', 'codex', -3, [(t(T('Explain how the invoice matcher picks a customer.', '인보이스 매처가 고객을 어떻게 고르는지 설명해 줘.')), 0)], steps=3),
        # ---- previous period (31 to 60 days ago): more long breaks and failing loops (what improved since)
        S('p01', 'claude', -57, [(t(T('Set up the trail list page.', '산책로 목록 페이지를 만들어 줘.')), 0), (t(T('Add sorting by distance.', '거리순 정렬도 넣어 줘.')), 95), (t(T('And by name.', '이름순도.')), 120)], steps=8, breaks=[1, 2], ctx0=40000),
        S('p02', 'claude', -53, [(t(T('Connect the map component.', '지도 컴포넌트를 연결해 줘.')), 0), (pq[0], 140)], steps=8, breaks=[1], fails=5, ctx0=45000,
          events=[ev('U2', 'R2a', 'agent_error', 'agent_side', 'major', 'U2', pq[0], 'resolved', pq[4])], verdict='agent_drift'),
        S('p10', 'claude', -46, [(t(T('Add a share button to the trip page.', '여행 페이지에 공유 버튼을 넣어 줘.')), 0), (pq[1], 6)], steps=6,
          events=[ev('U2', 'R2a', 'agent_error', 'agent_side', 'major', 'U2', pq[1], 'resolved', pq[5])], verdict='agent_drift'),
        S('p11', 'claude', -40, [(t(T('Make the search case-insensitive.', '검색에서 대소문자를 구분하지 않게 해 줘.')), 0), (pq[2], 9)], steps=6,
          events=[ev('U2', 'R6c', 'agent_error', 'agent_side', 'major', 'U2', pq[2], 'resolved', pq[6])], verdict='agent_drift'),
        S('p12', 'codex', -45, [(t(T('What does the retry limit do?', '재시도 한도는 무슨 역할을 해?')), 0), (pq[3], 5)], steps=5,
          events=[ev('U2', 'R3a', 'agent_error', 'agent_side', 'major', 'U2', pq[3], 'resolved', pq[7])], verdict='agent_drift'),
        S('p03', 'claude', -49, [(t(T('Add login with email links.', '이메일 링크로 로그인하게 해 줘.')), 0), (t(T('Now the logout button.', '이제 로그아웃 버튼.')), 75), (t(T('Style both.', '둘 다 꾸며 줘.')), 90)], steps=9, breaks=[1, 2], ctx0=50000),
        S('p04', 'claude', -44, [(t(T('Fix the flaky map test.', '자꾸 깨지는 지도 테스트를 고쳐 줘.')), 0)], steps=9, fails=6, ctx0=38000),
        S('p05', 'claude', -38, [(t(T('Add trail photos.', '산책로 사진을 넣어 줘.')), 0), (t(T('Resize them on upload.', '올릴 때 크기를 줄여 줘.')), 180)], steps=8, breaks=[1], ctx0=52000),
        S('p06', 'claude', -34, [(t(T('Write the onboarding text.', '처음 쓰는 사람용 안내 문구를 써 줘.')), 0)], steps=4),
        S('p07', 'codex', -50, [(t(T('Parse the bank CSV files.', '은행 CSV 파일을 읽게 해 줘.')), 0), (t(T('Handle the old format too.', '옛 형식도 처리해 줘.')), 100)], steps=7, breaks=[1], fails=5, ctx0=30000),
        S('p08', 'codex', -41, [(t(T('Speed up the sync job.', '동기화 작업을 빠르게 해 줘.')), 0)], steps=8, fails=4, ctx0=30000),
        S('p09', 'codex', -36, [(t(T('Add metrics for failed syncs.', '실패한 동기화에 지표를 붙여 줘.')), 0), (t(T('Export them to the dashboard.', '대시보드로 내보내 줘.')), 85)], steps=6, breaks=[1], ctx0=30000),
        S('p13', 'claude', -55, [(t(T('Add a trail difficulty badge.', '산책로 난이도 배지를 넣어 줘.')), 0), (t(T('The badge colours are swapped.', '배지 색이 서로 바뀌었어.')), 7)], steps=6,
          events=[ev('U2', 'R2a', 'agent_error', 'agent_side', 'major', 'U2', t(T('The badge colours are swapped.', '배지 색이 서로 바뀌었어.')), 'resolved', t(T('The agent swapped the badge colours for easy and hard trails.', '에이전트가 쉬움과 어려움 배지 색을 서로 바꿔 놓았습니다.')))], verdict='agent_drift'),
        S('p14', 'claude', -52, [(t(T('Show the trail length in miles too.', '산책로 길이를 마일로도 보여 줘.')), 0), (t(T('Miles are rounded to whole numbers now. Keep one decimal.', '마일이 정수로 반올림돼. 소수점 한 자리는 남겨.')), 7)], steps=6,
          events=[ev('U2', 'R2a', 'agent_error', 'agent_side', 'major', 'U2', t(T('Miles are rounded to whole numbers now. Keep one decimal.', '마일이 정수로 반올림돼. 소수점 한 자리는 남겨.')), 'resolved', t(T('The agent rounded the miles to whole numbers.', '에이전트가 마일을 정수로 반올림했습니다.')))], verdict='agent_drift'),
        S('p15', 'codex', -48, [(t(T('How is the invoice total computed?', '인보이스 합계는 어떻게 계산돼?')), 0), (t(T('I only asked. Why did you change total.go?', '물어보기만 했는데 total.go는 왜 고쳤어?')), 7)], steps=6,
          events=[ev('U2', 'R3a', 'agent_error', 'agent_side', 'major', 'U2', t(T('I only asked. Why did you change total.go?', '물어보기만 했는데 total.go는 왜 고쳤어?')), 'resolved', t(T('You asked how the total is computed; the agent edited total.go.', '합계 계산 방식을 물었는데, 에이전트가 total.go를 고쳤습니다.')))], verdict='agent_drift'),
        S('p16', 'claude', -56, [(t(T('Add a map legend.', '지도 범례를 넣어 줘.')), 0), (t(T('Thanks.', '고마워.')), 95)], steps=6, breaks=[1], ctx0=36000),
        S('p17', 'claude', -51, [(t(T('Cache the elevation tiles.', '고도 타일을 캐시해 줘.')), 0), (t(T('Thanks.', '고마워.')), 95)], steps=6, breaks=[], ctx0=36000),
        S('p18', 'codex', -47, [(t(T('Add a dry-run flag to the sync.', '동기화에 dry-run 옵션을 넣어 줘.')), 0), (t(T('Thanks.', '고마워.')), 95)], steps=6, breaks=[1], ctx0=36000),
        S('p19', 'claude', -43, [(t(T('Fix the footer links.', '푸터 링크를 고쳐 줘.')), 0), (t(T('Thanks.', '고마워.')), 95)], steps=6, breaks=[], ctx0=36000),
        S('p20', 'codex', -39, [(t(T('Log the sync duration.', '동기화 시간을 로그에 남겨 줘.')), 0), (t(T('Thanks.', '고마워.')), 95)], steps=6, breaks=[1], ctx0=36000),
        S('p21', 'claude', -35, [(t(T('Add an empty state to the trip list.', '여행 목록이 비었을 때 화면을 넣어 줘.')), 0), (t(T('Thanks.', '고마워.')), 95)], steps=6, breaks=[1], ctx0=36000),
        S('c13', 'claude', -11, [(t(T('Add keyboard shortcuts to the map.', '지도에 단축키를 넣어 줘.')), 0), (t(T('Looks good.', '좋아 보여.')), 6)], steps=5),
        S('c14', 'codex', -7, [(t(T('Retry the export when the bank API times out.', '은행 API가 시간 초과되면 내보내기를 다시 시도해 줘.')), 0), (t(T('Looks good.', '좋아 보여.')), 6)], steps=5),
    ]
    themes = [{'tool': 'claude', 'label': t(T('Leave existing migrations unchanged', '기존 마이그레이션은 그대로 두기')),
               'stated': [t(T('Keep the existing migration files as they are.', '기존 마이그레이션 파일은 그대로 둬.')),
                          t(T("Don't edit old migrations, add a new one if the schema changes.", '옛 마이그레이션은 고치지 말고, 스키마가 바뀌면 새 파일을 추가해.')),
                          t(T('Migrations that already ran stay untouched.', '이미 적용된 마이그레이션은 손대지 마.'))],
               'rule': t(T('Never edit a migration that already exists; add a new migration file instead.',
                           '이미 있는 마이그레이션은 고치지 말고, 바꿀 것이 있으면 새 마이그레이션 파일을 추가한다.'))}]
    return sess, themes


# ---------------------------------------------------------------- log writers
def sid(L, name):
    return str(uuid.UUID(hashlib.sha256(f'leeway-example:{L}:{name}'.encode()).hexdigest()[:32], version=4))


def iso(t):
    return t.strftime('%Y-%m-%dT%H:%M:%S.') + f'{t.microsecond // 1000:03d}Z'


AGENT_SAY = T(['Done. I changed the files listed above.', 'I updated the code and checked the page.', 'Here is what I changed.'],
              ['끝났습니다. 위 파일을 바꿨습니다.', '코드를 고치고 화면을 확인했습니다.', '바꾼 내용은 이렇습니다.'])
FILES = ['src/routes/trails.ts', 'src/components/TrailCard.tsx', 'src/components/Map.tsx', 'src/lib/distance.ts', 'src/pages/trip.tsx', 'db/schema.sql', 'src/api/client.ts', 'src/hooks/useTrails.ts']
CMDS = ['cat {f}', 'sed -n 1,80p {f}', 'grep -n export {f}', 'git diff --stat -- {f}']
XFILES = ['internal/sync/sync.go', 'internal/bank/client.go', 'internal/match/match.go', 'internal/report/csv.go', 'cmd/sync/main.go', 'internal/store/store.go']
XCMDS = ['cat {f}', 'sed -n 1,120p {f}', 'grep -n func {f}']


def write_claude(root, L, s, now, rng):
    sid_ = sid(L, s['name'])
    cwd = f'{HOME}/{PROJ_C}'
    d = Path(root) / 'claude' / ('-home-dev-' + PROJ_C)
    d.mkdir(parents=True, exist_ok=True)
    t = (now + dt.timedelta(days=s['day'])).replace(hour=9 + rng.randint(0, 9), minute=rng.randint(0, 59), second=0, microsecond=0)
    n = [0]
    lines = []

    def base(kind, ts):
        n[0] += 1
        return {'type': kind, 'sessionId': sid_, 'cwd': cwd, 'version': '2.1.40', 'timestamp': iso(ts),
                'uuid': str(uuid.UUID(hashlib.md5(f'{sid_}:{n[0]}'.encode()).hexdigest(), version=4)), 'entrypoint': 'cli'}
    o = base('user', t)
    o['message'] = {'role': 'user', 'content': '<system-reminder>\nAs you answer the user\'s questions, you can use the following context:\n'
                    f'Contents of {cwd}/CLAUDE.md (project instructions, checked into the codebase):\n\n{CLAUDE_MD[L]}\n</system-reminder>'}
    o['isMeta'] = True
    lines.append(o)
    for att in (skill_listing(), {'type': 'instructions', 'files': [{'path': f'{cwd}/CLAUDE.md', 'type': 'Project', 'content': CLAUDE_MD[L]}]}):
        x = base('attachment', t)
        x['attachment'] = att
        lines.append(x)
    ctx = s['ctx0']
    model = CLAUDE_MODEL
    for ti, (text, gap) in enumerate(s['turns']):
        if ti in s['breaks']: gap = max(gap, 70 + rng.randint(0, 120))   # a real break: longer than the 1 h cache
        t += dt.timedelta(minutes=gap or 0, seconds=20)
        expired = (ti in s['breaks']) or gap >= 60
        u = base('user', t)
        u['message'] = {'role': 'user', 'content': text}
        u['origin'] = {'kind': 'human'}
        lines.append(u)
        if s['switch'] and ti == len(s['turns']) - 1:
            model = 'claude-sonnet-5-5'
        steps = s['steps'] * 6 + (s['fails'] if ti == 0 else 0)
        for k in range(steps):
            t += dt.timedelta(seconds=rng.randint(15, 70))
            fail = ti == 0 and k < s['fails']
            cmd = 'npm test' if fail else rng.choice(CMDS).format(f=FILES[(k * 3 + ti) % len(FILES)]) + f'  # {k}'
            grow = rng.randint(1200, 3600)
            first = (k == 0)
            usage = {'input_tokens': rng.randint(4, 40), 'output_tokens': rng.randint(150, 900),
                     'cache_read_input_tokens': 0 if (first and (expired or ti == 0)) or (model != CLAUDE_MODEL and first) else ctx,
                     'cache_creation_input_tokens': (ctx + grow) if (first and (expired or ti == 0)) or (model != CLAUDE_MODEL and first) else grow}
            ctx += grow
            a = base('assistant', t)
            tid = f'toolu_{s["name"]}_{ti}_{k}'
            a['message'] = {'id': f'msg_{s["name"]}_{ti}_{k}', 'model': model, 'role': 'assistant',
                            'content': [{'type': 'tool_use', 'id': tid, 'name': 'Bash', 'input': {'command': cmd}}], 'usage': usage}
            lines.append(a)
            t += dt.timedelta(seconds=2)
            r = base('user', t)
            r['message'] = {'role': 'user', 'content': [{'type': 'tool_result', 'tool_use_id': tid, 'is_error': fail,
                                                         'content': 'Error: 2 tests failed\nExit code 1' if fail else 'ok'}]}
            lines.append(r)
        t += dt.timedelta(seconds=20)
        a = base('assistant', t)
        a['message'] = {'id': f'msg_{s["name"]}_{ti}_end', 'model': model, 'role': 'assistant',
                        'content': [{'type': 'text', 'text': rng.choice(AGENT_SAY[L])}],
                        'usage': {'input_tokens': 6, 'output_tokens': rng.randint(80, 300), 'cache_read_input_tokens': ctx, 'cache_creation_input_tokens': 400}}
        ctx += 400
        lines.append(a)
    p = d / f'{sid_}.jsonl'
    p.write_text(''.join(json.dumps(x, ensure_ascii=False) + '\n' for x in lines), encoding='utf-8')
    os.utime(p, (t.timestamp(), t.timestamp()))


def write_codex(root, L, s, now, rng):
    sid_ = sid(L, s['name'])
    cwd = f'{HOME}/{PROJ_X}'
    t = (now + dt.timedelta(days=s['day'])).replace(hour=9 + rng.randint(0, 9), minute=rng.randint(0, 59), second=0, microsecond=0)
    d = Path(root) / 'codex' / t.strftime('%Y/%m/%d')
    d.mkdir(parents=True, exist_ok=True)
    lines, n = [], [0]

    def add(kind, payload, ts):
        lines.append({'timestamp': iso(ts), 'type': kind, 'payload': payload, 'ordinal': n[0]})
        n[0] += 1
    add('session_meta', {'id': sid_, 'session_id': sid_, 'cwd': cwd, 'originator': 'codex_cli_rs', 'cli_version': '0.171.0', 'source': 'cli'}, t)
    add('event_msg', {'type': 'task_started', 'turn_attribution': {'turn_trigger': 'composer'}}, t)
    add('world_state', {'full': True, 'state': {'host_skills': {'version': 1, 'entries': {
        json.dumps(['host', 'host', f'{HOME}/.agents/skills/{n}/SKILL.md'], separators=(',', ':')): {'fingerprint': hashlib.sha256(n.encode()).hexdigest(), 'shown': True}
        for n in CODEX_SKILLS}}, 'agents_md': {'directory': cwd, 'text': AGENTS_MD[L]}}}, t)
    add('turn_context', {'model': CODEX_MODEL, 'cwd': cwd, 'effort': 'medium'}, t)
    add('response_item', {'type': 'message', 'role': 'user', 'content': [{'type': 'input_text', 'text': f'# AGENTS.md instructions for {cwd}\n\n<INSTRUCTIONS>\n{AGENTS_MD[L]}\n</INSTRUCTIONS>'}]}, t)
    add('response_item', {'type': 'message', 'role': 'user', 'content': [{'type': 'input_text', 'text': f'<environment_context>\n  <cwd>{cwd}</cwd>\n</environment_context>'}]}, t)
    ctx, c = s['ctx0'], 0
    for ti, (text, gap) in enumerate(s['turns']):
        if ti in s['breaks']: gap = max(gap, 45 + rng.randint(0, 90))   # longer than Codex's 30 min cache
        t += dt.timedelta(minutes=gap or 0, seconds=20)
        expired = (ti in s['breaks']) or gap >= 30
        if ti:
            add('event_msg', {'type': 'task_started', 'turn_attribution': {'turn_trigger': 'composer'}}, t)
        add('response_item', {'type': 'message', 'role': 'user', 'content': [{'type': 'input_text', 'text': text}]}, t)
        steps = s['steps'] * 5 + (s['fails'] if ti == 0 else 0)
        for k in range(steps):
            t += dt.timedelta(seconds=rng.randint(15, 60))
            fail = ti == 0 and k < s['fails']
            cmd = 'go test ./...' if fail else rng.choice(XCMDS).format(f=XFILES[(k * 5 + ti) % len(XFILES)]) + f'  # {k}'
            c += 1
            add('response_item', {'type': 'function_call', 'name': 'exec_command', 'arguments': json.dumps({'cmd': cmd}), 'call_id': f'c{c}'}, t)
            t += dt.timedelta(seconds=1)
            add('response_item', {'type': 'function_call_output', 'call_id': f'c{c}', 'output': ('Exit code: 1\nFAIL\tinvoice-sync/internal/sync' if fail else 'Process exited with code 0')}, t)
            grow = rng.randint(1200, 3800)
            cached = 0 if (k == 0 and (expired or ti == 0)) else ctx
            ctx += grow
            add('event_msg', {'type': 'token_count', 'info': {'last_token_usage': {'input_tokens': ctx, 'cached_input_tokens': cached,
                                                                                   'output_tokens': rng.randint(120, 700), 'reasoning_output_tokens': rng.randint(20, 200)},
                                                              'total_token_usage': {'input_tokens': c}}}, t)
        t += dt.timedelta(seconds=10)
        add('response_item', {'type': 'message', 'role': 'assistant', 'content': [{'type': 'output_text', 'text': rng.choice(AGENT_SAY[L])}]}, t)
    name = f'rollout-{lines[0]["timestamp"][:19].replace(":", "-")}-{sid_}.jsonl'
    (d / name).write_text(''.join(json.dumps(x, ensure_ascii=False) + '\n' for x in lines), encoding='utf-8')
    os.utime(d / name, (t.timestamp(), t.timestamp()))


def cmd_logs(out, L, now):
    sess, _ = scenario(L)
    rng = random.Random(20261123)
    for s in sess:
        (write_claude if s['tool'] == 'claude' else write_codex)(out, L, s, now, rng)


# ---------------------------------------------------------------- the stand-in judge (LEEWAY_JUDGE_CMD)
def answer(obj):
    return json.dumps({'text': '```json\n' + json.dumps(obj, ensure_ascii=False, indent=1) + '\n```',
                       'usage': {'input': 9000, 'output': 700, 'model': obj.pop('_model', '')}}, ensure_ascii=False)


def cmd_judge():
    req = json.loads(sys.stdin.read())
    L = os.environ.get('LEEWAY_EXAMPLE_LANG', 'en')
    sess, themes = scenario(L)
    prompt, system, tier = req['prompt'], req['system'], req['tier']
    model = CLAUDE_MODEL if tier.startswith('claude') else CODEX_MODEL
    if system.startswith('You group problems'):
        items = json.loads(prompt[prompt.index('{'):])['items']
        out = []
        for th in themes:
            if not tier.startswith(th['tool']):
                continue
            ids, stated = [], {}
            for it in items:
                words = [it.get('instruction') or ''] + list(it.get('person_said') or [])
                hit = next((q for q in th['stated'] if any(q in w for w in words if w)), None)
                if hit:
                    ids.append(it['id']); stated[it['id']] = hit
            if ids:
                out.append({'label': th['label'], 'events': ids, 'stated': stated, 'rule': th['rule']})
        print(answer({'themes': out, '_model': model}))
        return
    # the session is recognised by its whole first user turn, exactly as the excerpt writes it ("[U1] <text>" on its
    # own line), never by a part of it ("음" is inside "처음")
    hits = [x for x in sess if x['tool'] == tier.split(':')[0] and f"\n[U1] {x['turns'][0][0]}\n" in prompt]
    if len(hits) != 1:
        print(json.dumps({'text': f'{len(hits)} sessions match'})); sys.exit(2)
    s = hits[0]
    print(answer({'events': s['events'], 'defect_reports': [], 'weak_signals': [], 'intent_changes': [],
                  'no_user_reaction': s['verdict'] == 'undeterminable', 'verdict': s['verdict'],
                  'verdict_reason': 'synthetic example (hand-written judgment)', '_model': model}))


# ---------------------------------------------------------------- the frame around the report, and the check
FRAME = {
 'en': dict(TITLE='Leeway · example report', DESC='An example Leeway report made from made-up sessions: how the agents did, what to fix first, and what it cost.',
            NOTE_SHORT='Example · made-up data', NOTE_TAIL='sessions and judgments written by hand, not real logs or model results',
            INSTALL='Install', COPY='Copy', COPIED='Copied', INSTALL_UNIX='macOS and Linux', INSTALL_WIN='Windows (PowerShell)',
            INSTALL_THEN='Then ask Claude Code or Codex: “Make a Leeway report.”', IFRAME_TITLE='Example Leeway report',
            PRIVACY='Runs on your computer. The basic report sends nothing; the in-depth check asks first.'),
 'ko': dict(TITLE='Leeway · 예시 리포트', DESC='지어낸 세션으로 만든 Leeway 예시 리포트입니다. 에이전트가 요청대로 했는지, 무엇부터 고칠지, 비용이 얼마나 샜는지 보여 줍니다.',
            NOTE_SHORT='예시 · 지어낸 데이터', NOTE_TAIL='세션과 판정 모두 손으로 쓴 것으로, 실제 로그나 모델 판정 결과가 아닙니다',
            INSTALL='설치', COPY='복사', COPIED='복사됨', INSTALL_UNIX='macOS, Linux', INSTALL_WIN='Windows (PowerShell)',
            INSTALL_THEN='설치한 뒤 Claude Code나 Codex에 “Leeway 리포트 만들어줘”라고 말하면 됩니다.', IFRAME_TITLE='Leeway 예시 리포트',
            PRIVACY='내 컴퓨터에서 실행됩니다. 기본 리포트는 아무것도 보내지 않고, 정밀 점검은 보내기 전에 먼저 묻습니다.'),
}
REPO = 'https://github.com/Mango-Juice/agent-leeway'


def cmd_frame(out):
    tpl = (Path(__file__).parent / 'frame.html').read_text(encoding='utf-8')
    for L, page, rep, en, ko in (('en', 'index.html', 'report/en.html', './', 'ko/'), ('ko', 'ko/index.html', '../report/ko.html', '../', './')):
        v = dict(FRAME[L], LANG=L, REPO=REPO, REPORT=rep, EN_HREF=en, KO_HREF=ko, EN_CUR=' cur' if L == 'en' else '', KO_CUR=' cur' if L == 'ko' else '')
        v['NOTE_LONG'] = v['NOTE_SHORT'] + ' · ' + v['NOTE_TAIL']
        h = tpl
        for k, x in v.items():
            h = h.replace('{{' + k + '}}', x)
        assert '{{' not in h, page
        (Path(out) / page).parent.mkdir(parents=True, exist_ok=True)
        (Path(out) / page).write_text(h, encoding='utf-8')


CHECK = {'en': dict(fair='Fair', unfixed='It was never fixed.'), 'ko': dict(fair='보통', unfixed='끝내 고쳐지지 않았습니다.')}


def section(md, anchor):
    """The Markdown between <a id="anchor"> and the next anchor ("" when absent)."""
    i = md.find(f'<a id="{anchor}"></a>')
    if i < 0:
        return ''
    j = md.find('<a id="', i + 1)
    return md[i:j if j > 0 else len(md)]


def cmd_check(out, raw):
    """Fails the build when the example stops showing what site/README.md says it shows ('What the example must show')."""
    bad, verdicts = [], {}
    for L in ('en', 'ko'):
        html = (Path(out) / 'report' / f'{L}.html').read_text(encoding='utf-8')
        md = (Path(raw) / f'{L}.md').read_text(encoding='utf-8')
        sess, themes = scenario(L)
        th, c = themes[0], CHECK[L]
        i = html.index('<section class="sec verdict"')
        first = _html.unescape(re.sub(r'<[^>]+>', ' ', html[i:html.index('</section>', i)]))
        words = len(re.findall(r"[A-Za-z][A-Za-z'’.-]*", first)) if L == 'en' else len(re.findall(r'[가-힣]', first))
        table = [x for x in md.split('\n')[:10] if x.startswith('| ') and '**' in x]
        fix1, leak = section(md, 'f-drift-broke-chat'), section(md, 'f-leak-long-break')
        row = r'(\d+) of (\d+) → (\d+) of (\d+)' if L == 'en' else r'(\d+)개 중 (\d+)개 → (\d+)개 중 (\d+)개'
        counts = []  # each row as (rate then, rate now)
        for b in re.findall(r'<li class="cnt"><span>[^<]*</span><span class="cv"><b>([^<]*)</b>', html):
            m = re.fullmatch(row, b)
            if m:
                x = [int(v) for v in m.groups()]
                then, now = (x[0] / x[1], x[2] / x[3]) if L == 'en' else (x[1] / x[0], x[3] / x[2])
                counts.append((then, now))
        cards = re.findall(r'<p class="chg (\w+)">(.*?)</p>', html)
        label = {'en': ('vs prior 30 days', 'check'), 'ko': ('직전 30일 대비', '점검 대비')}[L]
        # what the stand-in judged: every session once, with its hand-written verdict
        by_key = {f"{x['tool']}:{sid(L, x['name'])}": x for x in sess}
        got = {}
        for line in (Path(raw) / f'{L}.results.jsonl').read_text(encoding='utf-8').splitlines():
            r = json.loads(line)
            if not r.get('error') and r.get('out'):
                got[r['key']] = (r.get('out') or {}).get('verdict')
        verdicts[L] = sorted(got.values())
        wrong = [x['name'] for k, x in by_key.items() if got.get(k) != x['verdict']]
        need = {
            'both parts checked, Fair on both': len(table) == 2 and all(f"**{c['fair']}**" in x for x in table),
            'overall verdict is Fair': c['fair'] in md.split('\n', 1)[0],
            'first fix is the theme': th['label'] in md.split('---')[0],
            'the theme was asked for in three sessions, in different words': all(q in fix1 for q in th['stated']) and len(set(th['stated'])) == 3,
            'the theme has a draft rule': th['rule'] in fix1,
            'rule already in CLAUDE.md is a finding': FILE_RULE[L] in html,
            'cost leak in dollars (resuming after long breaks)': re.search(r'\$\d+\.\d\d', leak) is not None,
            'going-well section': 'id="s-good"' in html,
            'one problem was never fixed': md.count(c['unfixed']) == 1,
            'evidence dialog': 'data-ev=' in html,
            'a setup fix: the skill list over its budget, as Fix 3': re.search(r'<a id="f-setup-skills-claude"></a>\n## [^\n:]*3[^\n:]*3: ', md) is not None,
            'your setup: three rows, one over the line': html.count('<li class="ok"><span class="ci"') == 2 and html.count('<li class="over"><span class="ci"') == 1,
            'in-depth card: both rates, no word (within normal swings), against the first check': len(cards) == 2 and cards[0][0] == 'neutral' and label[1] in cards[0][1],
            'cost card: Better, against the previous 30 days': len(cards) == 2 and cards[1][0] == 'better' and label[0] in cards[1][1],
            'some kinds of problem fewer than at the first check, some more': any(b < a for a, b in counts[1:]) and any(b > a for a, b in counts[1:]),
            f'first screen within budget ({words} of {75 if L == "en" else 155})': words <= (75 if L == 'en' else 155),
            'every session judged once, with its hand-written verdict' + (f' (wrong: {", ".join(wrong)})' if wrong else ''): not wrong and len(got) == len(sess),
            'no real paths': '/Users/' not in html and '/home/dev' not in html.replace('/home/dev/trail-planner', '').replace('/home/dev/invoice-sync', ''),  # privacy-ok: the check for real paths
            'no external assets': 'src="http' not in html and "url(http" not in html,
        }
        bad += [f'{L}: {k}' for k, ok in need.items() if not ok]
    if verdicts['en'] != verdicts['ko']:
        bad.append('the English and Korean builds judged differently')
    for p in Path(out).rglob('*'):  # the site is public: everything readable by everyone
        if p.stat().st_mode & (0o555 if p.is_dir() else 0o444) != (0o555 if p.is_dir() else 0o444):
            bad.append(f'not world-readable: {p.relative_to(out)}')
    if bad:
        sys.exit('example check failed:\n  ' + '\n  '.join(bad))
    print('example check: ok')


if __name__ == '__main__':
    if sys.argv[1] == 'logs':
        out, L = sys.argv[2], sys.argv[sys.argv.index('--lang') + 1]
        now = dt.datetime.fromisoformat(sys.argv[sys.argv.index('--now') + 1].replace('Z', '+00:00')) if '--now' in sys.argv else dt.datetime.now(dt.timezone.utc)
        cmd_logs(out, L, now)
    elif sys.argv[1] == 'judge':
        cmd_judge()
    elif sys.argv[1] == 'frame':
        cmd_frame(sys.argv[2])
    elif sys.argv[1] == 'check':
        cmd_check(sys.argv[2], sys.argv[3])
