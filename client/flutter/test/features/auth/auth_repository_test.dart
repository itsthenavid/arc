import 'package:arc/features/auth/data/api_exception.dart';
import 'package:arc/features/auth/data/auth_api.dart';
import 'package:arc/features/auth/data/auth_repository.dart';
import 'package:arc/features/auth/data/token_store.dart';
import 'package:arc/features/auth/domain/auth_models.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test('ensureActiveSession deduplicates concurrent refresh calls', () async {
    final store = _MemoryStore(
      session: _expiredSession(refreshToken: 'refresh-1'),
      user: _user(),
    );

    var refreshCalls = 0;
    final api = _FakeApi(
      onRefresh: ({refreshToken, required rememberMe}) async {
        refreshCalls++;
        await Future<void>.delayed(const Duration(milliseconds: 40));
        return _activeSession(refreshToken: 'refresh-2');
      },
      onMe: (accessToken) async => _user(),
    );

    final repo = DefaultAuthRepository(
      api: api,
      store: store,
      webCookieMode: false,
    );

    final results = await Future.wait(
      List.generate(
        6,
        (_) => repo.ensureActiveSession(skew: const Duration(days: 1)),
      ),
    );

    expect(refreshCalls, 1);
    expect(results.every((s) => s.accessToken == 'access-active'), isTrue);
    expect((await store.readSession())?.refreshToken, 'refresh-2');
  });

  test('bootstrap clears local state when refresh is rejected', () async {
    final store = _MemoryStore(
      session: _expiredSession(refreshToken: 'refresh-1'),
      user: _user(),
    );

    final api = _FakeApi(
      onRefresh: ({refreshToken, required rememberMe}) async {
        throw const ApiException(
          statusCode: 401,
          code: 'session_not_active',
          kind: ApiErrorKind.unauthorized,
          message: 'session not active',
        );
      },
      onMe: (accessToken) async => _user(),
    );

    final repo = DefaultAuthRepository(
      api: api,
      store: store,
      webCookieMode: false,
    );

    final result = await repo.bootstrap();

    expect(result.authenticated, isFalse);
    expect(await store.readSession(), isNull);
    expect(await store.readUser(), isNull);
  });

  test('login persists user and session', () async {
    final store = _MemoryStore();

    final api = _FakeApi(
      onLogin:
          ({username, email, required password, required rememberMe}) async {
            return ArcAuthBundle(
              user: _user(),
              session: _activeSession(refreshToken: 'refresh-new'),
            );
          },
      onRefresh: ({refreshToken, required rememberMe}) async =>
          _activeSession(refreshToken: refreshToken),
      onMe: (accessToken) async => _user(),
    );

    final repo = DefaultAuthRepository(
      api: api,
      store: store,
      webCookieMode: false,
    );

    final bundle = await repo.login(
      username: 'user_one',
      password: 'very-strong-password',
      rememberMe: true,
    );

    expect(bundle.user.id, 'user_1');
    expect((await store.readSession())?.sessionId, 'sess-active');
    expect((await store.readUser())?.id, 'user_1');
  });
}

class _FakeApi implements AuthApiClient {
  _FakeApi({
    Future<ArcAuthBundle> Function({
      String? username,
      String? email,
      required String password,
      required bool rememberMe,
    })?
    onLogin,
    Future<ArcAuthBundle> Function({
      required String inviteToken,
      String? username,
      String? email,
      required String password,
      required bool rememberMe,
    })?
    onConsumeInvite,
    Future<ArcSession> Function({
      String? refreshToken,
      required bool rememberMe,
    })?
    onRefresh,
    Future<ArcUser> Function(String accessToken)? onMe,
    Future<void> Function(String accessToken)? onLogout,
    Future<void> Function(String accessToken)? onLogoutAll,
  }) : _onLogin = onLogin,
       _onConsumeInvite = onConsumeInvite,
       _onRefresh = onRefresh,
       _onMe = onMe,
       _onLogout = onLogout,
       _onLogoutAll = onLogoutAll;

  final Future<ArcAuthBundle> Function({
    String? username,
    String? email,
    required String password,
    required bool rememberMe,
  })?
  _onLogin;
  final Future<ArcAuthBundle> Function({
    required String inviteToken,
    String? username,
    String? email,
    required String password,
    required bool rememberMe,
  })?
  _onConsumeInvite;
  final Future<ArcSession> Function({
    String? refreshToken,
    required bool rememberMe,
  })?
  _onRefresh;
  final Future<ArcUser> Function(String accessToken)? _onMe;
  final Future<void> Function(String accessToken)? _onLogout;
  final Future<void> Function(String accessToken)? _onLogoutAll;

  @override
  Future<ArcAuthBundle> consumeInvite({
    required String inviteToken,
    String? username,
    String? email,
    required String password,
    required bool rememberMe,
  }) async {
    final onConsumeInvite = _onConsumeInvite;
    if (onConsumeInvite == null) {
      throw UnimplementedError();
    }
    return onConsumeInvite(
      inviteToken: inviteToken,
      username: username,
      email: email,
      password: password,
      rememberMe: rememberMe,
    );
  }

  @override
  Future<ArcAuthBundle> login({
    String? username,
    String? email,
    required String password,
    required bool rememberMe,
  }) async {
    final onLogin = _onLogin;
    if (onLogin == null) {
      throw UnimplementedError();
    }
    return onLogin(
      username: username,
      email: email,
      password: password,
      rememberMe: rememberMe,
    );
  }

  @override
  Future<void> logout({required String accessToken}) async {
    await _onLogout?.call(accessToken);
  }

  @override
  Future<void> logoutAll({required String accessToken}) async {
    await _onLogoutAll?.call(accessToken);
  }

  @override
  Future<ArcInvite> createInvite({
    required String accessToken,
    int maxUses = 1,
    Duration? ttl,
  }) {
    throw UnimplementedError();
  }

  @override
  Future<ArcUser> me({required String accessToken}) async {
    final onMe = _onMe;
    if (onMe == null) {
      throw UnimplementedError();
    }
    return onMe(accessToken);
  }

  @override
  Future<ArcSession> refresh({
    String? refreshToken,
    required bool rememberMe,
  }) async {
    final onRefresh = _onRefresh;
    if (onRefresh == null) {
      throw UnimplementedError();
    }
    return onRefresh(refreshToken: refreshToken, rememberMe: rememberMe);
  }
}

class _MemoryStore implements AuthStore {
  _MemoryStore({
    ArcSession? session,
    ArcUser? user,
    ProfileDraft? profileDraft,
    String? inviteToken,
  }) : _session = session,
       _user = user,
       _profileDraft = profileDraft,
       _inviteToken = inviteToken;

  ArcSession? _session;
  ArcUser? _user;
  ProfileDraft? _profileDraft;
  String? _inviteToken;

  @override
  Future<void> clearAll() async {
    _session = null;
    _user = null;
    _profileDraft = null;
    _inviteToken = null;
  }

  @override
  Future<void> clearInviteToken() async {
    _inviteToken = null;
  }

  @override
  Future<void> clearProfileDraft() async {
    _profileDraft = null;
  }

  @override
  Future<void> clearSession() async {
    _session = null;
  }

  @override
  Future<void> clearUser() async {
    _user = null;
  }

  @override
  Future<String?> readInviteToken() async {
    return _inviteToken;
  }

  @override
  Future<ProfileDraft?> readProfileDraft() async {
    return _profileDraft;
  }

  @override
  Future<ArcSession?> readSession() async {
    return _session;
  }

  @override
  Future<ArcUser?> readUser() async {
    return _user;
  }

  @override
  Future<void> writeInviteToken(String token) async {
    _inviteToken = token;
  }

  @override
  Future<void> writeProfileDraft(ProfileDraft draft) async {
    _profileDraft = draft;
  }

  @override
  Future<void> writeSession(ArcSession session) async {
    _session = session;
  }

  @override
  Future<void> writeUser(ArcUser user) async {
    _user = user;
  }
}

ArcUser _user() {
  return ArcUser(
    id: 'user_1',
    username: 'user_one',
    email: 'user@example.com',
    createdAt: DateTime.utc(2026, 1, 1),
    displayName: 'User One',
  );
}

ArcSession _expiredSession({required String refreshToken}) {
  return ArcSession(
    sessionId: 'sess-expired',
    accessToken: 'access-expired',
    accessExpiresAt: DateTime.now().toUtc().subtract(
      const Duration(minutes: 5),
    ),
    refreshToken: refreshToken,
    refreshExpiresAt: DateTime.now().toUtc().add(const Duration(days: 7)),
    rememberMe: true,
  );
}

ArcSession _activeSession({required String? refreshToken}) {
  return ArcSession(
    sessionId: 'sess-active',
    accessToken: 'access-active',
    accessExpiresAt: DateTime.now().toUtc().add(const Duration(minutes: 15)),
    refreshToken: refreshToken,
    refreshExpiresAt: DateTime.now().toUtc().add(const Duration(days: 30)),
    rememberMe: true,
  );
}
