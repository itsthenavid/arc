import 'package:arc/core/platform/platform_info.dart';
import 'package:arc/features/auth/domain/auth_models.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:shared_preferences/shared_preferences.dart';

abstract class AuthStore {
  Future<ArcSession?> readSession();
  Future<void> writeSession(ArcSession session);
  Future<void> clearSession();

  Future<ArcUser?> readUser();
  Future<void> writeUser(ArcUser user);
  Future<void> clearUser();

  Future<ProfileDraft?> readProfileDraft();
  Future<void> writeProfileDraft(ProfileDraft draft);
  Future<void> clearProfileDraft();

  Future<String?> readInviteToken();
  Future<void> writeInviteToken(String token);
  Future<void> clearInviteToken();

  Future<void> clearAll();
}

class DeviceAuthStore implements AuthStore {
  DeviceAuthStore({
    required PlatformInfo platform,
    FlutterSecureStorage? secureStorage,
    Future<SharedPreferences>? sharedPreferences,
  }) : _platform = platform,
       _secureStorage = secureStorage ?? const FlutterSecureStorage(),
       _prefsFuture = sharedPreferences ?? SharedPreferences.getInstance();

  static const _kSession = 'arc.auth.session';
  static const _kUser = 'arc.auth.user';
  static const _kProfileDraft = 'arc.auth.profile_draft';
  static const _kInviteToken = 'arc.auth.invite_token';

  final PlatformInfo _platform;
  final FlutterSecureStorage _secureStorage;
  final Future<SharedPreferences> _prefsFuture;

  SharedPreferences? _prefs;

  Future<SharedPreferences> _prefsInstance() async {
    return _prefs ??= await _prefsFuture;
  }

  bool get _sessionInPrefs => _platform.isWeb;

  @override
  Future<ArcSession?> readSession() async {
    final raw = await (_sessionInPrefs
        ? _readFromPrefs(_kSession)
        : _readFromSecure(_kSession));
    if (raw == null || raw.isEmpty) {
      return null;
    }
    return ArcSession.decode(raw);
  }

  @override
  Future<void> writeSession(ArcSession session) async {
    final storedSession = _sessionInPrefs
        ? session.copyWith(refreshToken: null)
        : session;
    await (_sessionInPrefs
        ? _writeToPrefs(_kSession, storedSession.encode())
        : _writeToSecure(_kSession, storedSession.encode()));
  }

  @override
  Future<void> clearSession() async {
    await (_sessionInPrefs
        ? _removeFromPrefs(_kSession)
        : _removeFromSecure(_kSession));
  }

  @override
  Future<ArcUser?> readUser() async {
    final raw = await _readFromPrefs(_kUser);
    if (raw == null || raw.isEmpty) {
      return null;
    }
    return ArcUser.decode(raw);
  }

  @override
  Future<void> writeUser(ArcUser user) {
    return _writeToPrefs(_kUser, user.encode());
  }

  @override
  Future<void> clearUser() {
    return _removeFromPrefs(_kUser);
  }

  @override
  Future<ProfileDraft?> readProfileDraft() async {
    final raw = await _readFromPrefs(_kProfileDraft);
    if (raw == null || raw.isEmpty) {
      return null;
    }
    return ProfileDraft.decode(raw);
  }

  @override
  Future<void> writeProfileDraft(ProfileDraft draft) {
    if (draft.isEmpty) {
      return clearProfileDraft();
    }
    return _writeToPrefs(_kProfileDraft, draft.encode());
  }

  @override
  Future<void> clearProfileDraft() {
    return _removeFromPrefs(_kProfileDraft);
  }

  @override
  Future<String?> readInviteToken() {
    return _readFromPrefs(_kInviteToken);
  }

  @override
  Future<void> writeInviteToken(String token) {
    final value = token.trim();
    if (value.isEmpty) {
      return clearInviteToken();
    }
    return _writeToPrefs(_kInviteToken, value);
  }

  @override
  Future<void> clearInviteToken() {
    return _removeFromPrefs(_kInviteToken);
  }

  @override
  Future<void> clearAll() async {
    await Future.wait<void>([
      clearSession(),
      clearUser(),
      clearProfileDraft(),
      clearInviteToken(),
    ]);
  }

  Future<String?> _readFromSecure(String key) async {
    try {
      return await _secureStorage.read(key: key);
    } catch (_) {
      return null;
    }
  }

  Future<void> _writeToSecure(String key, String value) async {
    try {
      await _secureStorage.write(key: key, value: value);
    } catch (_) {
      await _writeToPrefs(key, value);
    }
  }

  Future<void> _removeFromSecure(String key) async {
    try {
      await _secureStorage.delete(key: key);
    } catch (_) {
      // Ignore secure-store failures during logout/bootstrap cleanup.
    }
  }

  Future<String?> _readFromPrefs(String key) async {
    final prefs = await _prefsInstance();
    return prefs.getString(key);
  }

  Future<void> _writeToPrefs(String key, String value) async {
    final prefs = await _prefsInstance();
    await prefs.setString(key, value);
  }

  Future<void> _removeFromPrefs(String key) async {
    final prefs = await _prefsInstance();
    await prefs.remove(key);
  }
}
